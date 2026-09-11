package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"heimdall/internal/adapters/herdr"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"sort"
	"time"
)

// AgentService polls the live agent inventory of every active Herdr-bound
// session socket and records epoch-scoped agent observations. Observations are
// inventory evidence only; task attribution is derived by readers, never by
// this recorder.
type AgentService struct {
	Store     *store.Store
	Herdr     herdr.Adapter
	PollEvery time.Duration
}

func (s AgentService) Run(ctx context.Context, clock func() time.Time) {
	interval := s.PollEvery
	if interval <= 0 {
		interval = 2 * time.Second
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		s.poll(ctx, clock().UTC())
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

type agentSource struct{ socket, epoch, host string }

// sources returns the distinct epoch-checked Herdr sockets of active session
// bindings. Binding is the explicit opt-in; nothing else confers authority to
// inspect a Herdr session.
func agentSources(st model.State) []agentSource {
	seen := map[agentSource]bool{}
	out := []agentSource{}
	for surface, head := range st.SessionHeads {
		b := st.SessionBindings[head]
		if !b.Active || b.Locator == nil || b.Locator.Adapter != "herdr" || st.SessionHeads[b.SurfaceID] != head || b.SurfaceID != surface {
			continue
		}
		src := agentSource{socket: b.Locator.SessionID, epoch: b.Locator.SourceEpoch, host: b.Locator.Host}
		if !seen[src] {
			seen[src] = true
			out = append(out, src)
		}
	}
	return out
}

func (s AgentService) poll(ctx context.Context, now time.Time) {
	if s.Store == nil {
		return
	}
	st, err := s.Store.State(ctx)
	if err != nil {
		return
	}
	for _, src := range agentSources(st) {
		s.pollSource(ctx, src, now)
	}
}

func (s AgentService) pollSource(ctx context.Context, src agentSource, now time.Time) {
	ioctx, stop := context.WithTimeout(ctx, 3*time.Second)
	list, err := s.Herdr.Agents(ioctx, src.socket, src.epoch, src.host)
	stop()
	if err != nil {
		// An unreachable or re-epoch'd Herdr records nothing; the last
		// observations stay attributed to their original epoch.
		return
	}
	raw, err := json.Marshal(list)
	if err != nil {
		return
	}
	sum := sha256.Sum256(append([]byte(src.epoch), raw...))
	id := "agent-sync-" + hex.EncodeToString(sum[:8])
	_, _ = s.Store.Transact(ctx, id, "observer:herdr", raw, now, func(fresh model.State) (store.Change, error) {
		events := []store.Pending{}
		present := map[string]bool{}
		for _, info := range list {
			cwd := info.Cwd
			if cwd == "" {
				cwd = info.ForegroundCwd
			}
			r := model.AgentRecord{Version: 1, Adapter: "herdr", Host: src.host, SourceEpoch: src.epoch,
				SessionID: src.socket, WorkspaceID: info.WorkspaceID, PaneID: info.PaneID, TabID: info.TabID,
				TerminalID: info.TerminalID, Cwd: cwd, Agent: info.Agent, AgentSessionID: info.AgentSession.Value,
				Status: info.AgentStatus, Title: info.TerminalTitle, StateSeq: info.StateChangeSeq, Active: true, At: now}
			key := r.ContainerKey()
			present[key] = true
			if old, ok := fresh.AgentHeads[key]; !ok || !old.Active || old.StateSeq != r.StateSeq || old.Status != r.Status ||
				old.AgentSessionID != r.AgentSessionID || old.WorkspaceID != r.WorkspaceID || old.TabID != r.TabID ||
				old.TerminalID != r.TerminalID || old.Cwd != r.Cwd || old.Agent != r.Agent || old.Title != r.Title {
				events = append(events, store.Pending{Subject: "agent", Verb: "observed", EntityID: key, Payload: r})
			}
		}
		for key, old := range fresh.AgentHeads {
			if old.SourceEpoch != src.epoch || old.SessionID != src.socket || old.Host != src.host || !old.Active || present[key] {
				continue
			}
			detached := old
			detached.Active, detached.At = false, now
			events = append(events, store.Pending{Subject: "agent", Verb: "detached", EntityID: key, Payload: detached})
		}
		sort.Slice(events, func(i, j int) bool { return events[i].EntityID < events[j].EntityID })
		return store.Change{Revision: fresh.Revision, Events: events}, nil
	})
}

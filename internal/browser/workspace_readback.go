package browser

import (
	"context"
	"fmt"
	"heimdall/internal/model"
	"time"
)

type observationDemand struct {
	After             int64
	Epoch, Connection string
	Refs              []model.BrowserActionRef
	Started           time.Time
}

// ObserveWorkspace asks the existing browser poll/readback protocol for a fresh
// census of settled surfaces. The demand is bounded daemon memory: no action,
// outbox operation, redispatch or replayable execution authority is created.
func (s *Service) ObserveWorkspace(ctx context.Context, target string, surfaces []string, now time.Time) error {
	if s.Runtime == nil || !model.ValidID(target) || len(surfaces) > 32 {
		return fmt.Errorf("bounded browser observation runtime required")
	}
	st, err := s.Store.State(ctx)
	if err != nil {
		return err
	}
	demands := map[string]observationDemand{}
	for _, a := range st.Actions {
		b := a.Intent.Browser
		if b == nil || a.Intent.Target != target || !model.Contains(surfaces, a.Intent.SurfaceID) {
			continue
		}
		p := st.Browsers[b.Profile]
		if !p.Paired || p.Epoch != b.Epoch || p.VerificationProtocol != 1 {
			continue
		}
		d := demands[p.ID]
		d.Epoch, d.Connection, d.After = p.Epoch, p.Connection, st.LastEventID
		d.Refs = append(d.Refs, *a.BrowserRef())
		if len(d.Refs) > 128 {
			return fmt.Errorf("browser observation exceeds bounded action reference census")
		}
		demands[p.ID] = d
	}
	if len(demands) == 0 {
		return nil
	}
	s.Runtime.mu.Lock()
	started := s.Runtime.Clock()
	if s.Runtime.observations == nil {
		s.Runtime.observations = map[string]observationDemand{}
	}
	for id, d := range demands {
		if len(s.Runtime.observations) >= 128 { // Expire old request-only state before admitting more.
			for key, old := range s.Runtime.observations {
				if started.Sub(old.Started) >= 5*time.Second {
					delete(s.Runtime.observations, key)
				}
			}
			if len(s.Runtime.observations) >= 128 {
				s.Runtime.mu.Unlock()
				return fmt.Errorf("browser observation capacity reached")
			}
		}
		d.Started = started
		s.Runtime.observations[id] = d
	}
	s.Runtime.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		current, err := s.Store.State(ctx)
		if err != nil {
			return err
		}
		s.Runtime.mu.Lock()
		at := s.observationTime(now)
		ready := true
		for id, d := range demands {
			p := current.Browsers[id]
			if p.Epoch != d.Epoch || p.Connection != d.Connection || !s.fresh(p, d.After, at) {
				ready = false
			}
		}
		s.Runtime.mu.Unlock()
		if ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("fresh browser observation unavailable: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

// RecoveryFresh includes the live daemon's monotonic lease, not just persisted
// timestamps. No lease survives restart, an uninitialized runtime or reconnect.
func (s *Service) RecoveryFresh(p model.BrowserProfile, after int64, now time.Time) bool {
	if s.Runtime == nil {
		return false
	}
	s.Runtime.mu.Lock()
	defer s.Runtime.mu.Unlock()
	return s.fresh(p, after, now)
}

// observationTime samples the observation clock itself. Measuring elapsed time
// from a later demand start and adding it to the caller's timestamp loses the
// interval before that start and can make fresh readback appear to be future data.
func (s *Service) observationTime(now time.Time) time.Time {
	at := s.Runtime.Clock()
	if now.After(at) {
		return now
	}
	return at
}

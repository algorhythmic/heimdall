package workspace

import (
	"context"
	"fmt"
	"heimdall/internal/adapters/hyprland"
	"heimdall/internal/model"
	"reflect"
)

func applicationSession(st model.State, a model.ActionRecord) *model.SessionBinding {
	id := a.Intent.Native.SessionBindingID
	if a.Intent.Native.Launch != nil {
		id = a.Intent.Native.Launch.SessionBindingID
	}
	if id == "" {
		return nil
	}
	b := st.SessionBindings[id]
	return &b
}

func (s *OperationService) observeApplicationSession(ctx context.Context, st model.State, a model.ActionRecord, o *model.ActionObservation) error {
	b := applicationSession(st, a)
	if b == nil {
		return nil
	}
	if s.Previews == nil || s.Previews.Herdr == nil || !b.Active || b.Locator == nil || b.Herdr == nil || st.SessionHeads[a.Intent.SurfaceID] != b.ID {
		return fmt.Errorf("current Herdr observation unavailable")
	}
	observed, err := s.Previews.Herdr.Observe(ctx, b.Locator.SessionID, b.Locator.PaneID, b.Locator.SourceEpoch)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(observed.Locator, *b.Locator) || !reflect.DeepEqual(observed.Herdr, *b.Herdr) {
		return fmt.Errorf("Herdr session or pane process changed")
	}
	o.Native.SessionBindingID, o.Native.SessionDigest = b.ID, model.ApplicationSessionDigest(*b)
	o.Digest = model.ContentDigest(o.Native)
	return nil
}

func launchAbsent(st model.State, a model.ActionRecord, o hyprland.Status) error {
	b := st.ViewportBindings[a.Intent.Native.Launch.PreviousViewport]
	for _, w := range o.Snapshot.Windows {
		if w.Class == model.ApplicationClass(a.Intent.AttemptID) || (b.Active && b.Window != nil && w.Identity == *b.Window) {
			return fmt.Errorf("launch candidate or previous owned window already exists")
		}
	}
	return nil
}

func (s *OperationService) observeLaunch(st model.State, a model.ActionRecord, observed hyprland.Status, o *model.ActionObservation) {
	r := o.Native
	r.FocusKnown, r.FocusedWindow = false, nil
	if s.Applications == nil {
		return
	}
	for _, w := range observed.Snapshot.Windows {
		if w.Class == model.ApplicationClass(a.Intent.AttemptID) {
			r.LaunchCandidates++
		}
	}
	if a.Report != nil && a.Report.Native != nil && a.Report.Native.Process != nil && r.LaunchCandidates == 1 {
		pin := *a.Report.Native.Process
		p, err := s.Applications.Process(pin.PID)
		if err == nil && p == pin {
			for _, w := range observed.Snapshot.Windows {
				if w.PID != p.PID || w.Class != model.ApplicationClass(a.Intent.AttemptID) {
					continue
				}
				ownedElsewhere := false
				for surface, id := range st.ViewportHeads {
					b := st.ViewportBindings[id]
					if surface != a.Intent.SurfaceID && b.Active && b.Window != nil && *b.Window == w.Identity {
						ownedElsewhere = true
					}
				}
				if ownedElsewhere {
					continue
				}
				copy := w
				r.Process, r.Window = &p, &copy
				for _, ws := range observed.Snapshot.Workspaces {
					if ws.ID == w.WorkspaceID {
						r.Workspace = ws.Name
					}
				}
			}
		}
	}
	o.Digest = model.ContentDigest(r)
}

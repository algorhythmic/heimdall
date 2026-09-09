package browser

import (
	"context"
	"fmt"
	"heimdall/internal/model"
	"time"
)

// ObservationLease is an in-process read pin used by the compositor join. Its
// Check method needs no browser mutex while the store writer is held.
type ObservationLease struct {
	Profile  model.BrowserProfile
	runtime  string
	began    time.Time
	deadline time.Time
	clock    func() time.Time
}

func (s Service) Lease(ctx context.Context, profile string) (ObservationLease, error) {
	if s.Runtime == nil {
		return ObservationLease{}, fmt.Errorf("browser runtime unavailable")
	}
	s.Runtime.mu.Lock()
	defer s.Runtime.mu.Unlock()
	st, err := s.Store.State(ctx)
	if err != nil {
		return ObservationLease{}, err
	}
	p := st.Browsers[profile]
	if !p.Paired || !s.fresh(p, 0, s.Runtime.Clock().UTC()) {
		return ObservationLease{}, fmt.Errorf("fresh challenged browser readback unavailable")
	}
	stamp := s.Runtime.reads[profile]
	return ObservationLease{Profile: p, runtime: s.Store.RuntimeID(), began: stamp.At, deadline: stamp.At.Add(5 * time.Second), clock: s.Runtime.Clock}, nil
}
func (l ObservationLease) Check(st model.State, runtime string) error {
	if l.clock == nil {
		return fmt.Errorf("browser observation lease missing")
	}
	now := l.clock()
	if runtime != l.runtime || now.Before(l.began) || !now.Before(l.deadline) || model.ContentDigest(st.Browsers[l.Profile.ID]) != model.ContentDigest(l.Profile) {
		return fmt.Errorf("browser observation changed or expired")
	}
	return nil
}

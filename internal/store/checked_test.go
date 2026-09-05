package store

import (
	"context"
	"errors"
	"heimdall/internal/model"
	"testing"
	"time"
)

func TestCheckedTransactionGuardsCachedResultsAndCommit(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	denied := errors.New("expired or revoked")
	build := func(st model.State) (Change, error) {
		return Change{Revision: st.Revision + 1, Result: map[string]string{"receipt": "private"}}, nil
	}
	_, err = s.TransactChecked(ctx, "first", "cli", []byte("request"), time.Now(), func(st model.State) error {
		if st.Revision > 0 {
			return denied
		}
		return nil
	}, build)
	if !errors.Is(err, denied) {
		t.Fatal("post-build authorization not enforced", err)
	}
	st, _ := s.State(ctx)
	if st.LastEventID != 0 || st.Revision != 0 {
		t.Fatal("denied transaction left events")
	}
	if _, err = s.TransactChecked(ctx, "first", "cli", []byte("request"), time.Now(), nil, build); err != nil {
		t.Fatal(err)
	}
	if _, err = s.TransactChecked(ctx, "first", "cli", []byte("request"), time.Now(), func(model.State) error { return denied }, build); !errors.Is(err, denied) {
		t.Fatal("cached result bypassed authority", err)
	}
}

package store

import (
	"encoding/json"
	"heimdall/internal/model"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestUIProgressGoldenAuthorityAndLegacyRefusal(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/progress/events-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	if err := json.Unmarshal(raw, &events); err != nil {
		t.Fatal(err)
	}
	st, count := model.Empty(), 0
	for _, e := range events {
		if e.Subject == "progress" && e.Verb == "reviewed" {
			var review model.ProgressReview
			json.Unmarshal(e.Payload, &review)
			if review.Version == 2 {
				count++
				bad := func(mutate func(*model.ProgressReview)) {
					t.Helper()
					r := model.Clone(review)
					mutate(&r)
					broken := e
					broken.Actor = r.Actor
					broken.Payload, _ = json.Marshal(r)
					before := model.Clone(st)
					if err := Apply(&st, broken); err == nil {
						t.Fatal("invalid UI review authority accepted", string(broken.Payload))
					}
					if !reflect.DeepEqual(before, st) {
						t.Fatal("rejected review changed state")
					}
				}
				bad(func(r *model.ProgressReview) { r.Version = 1; r.Actor = "cli" })
				bad(func(r *model.ProgressReview) { r.Actor = "cli" })
				bad(func(r *model.ProgressReview) { r.Authority = nil })
				bad(func(r *model.ProgressReview) { r.Authority.Target = "private" })
				bad(func(r *model.ProgressReview) { r.Authority.ResourceIDs = []string{} })
				bad(func(r *model.ProgressReview) { r.Authority.ExpiresAt = r.At })
				bad(func(r *model.ProgressReview) { r.Authority.SessionID = strings.Repeat("f", 32) })
				// Even null is a new field that strict v1 must refuse.
				var fields map[string]any
				json.Unmarshal(e.Payload, &fields)
				fields["version"], fields["actor"], fields["authority"] = 1, "cli", nil
				broken := e
				broken.Actor = "cli"
				broken.Payload, _ = json.Marshal(fields)
				if err := Apply(&st, broken); err == nil {
					t.Fatal("legacy payload accepted authority null")
				}
			}
		}
		if err := Apply(&st, e); err != nil {
			t.Fatal(e.ID, err)
		}
	}
	if count != 3 || len(st.Decisions) != 1 || st.Tasks["workspace"].Task.Status != "active" {
		t.Fatal("wrong UI review replay state", count)
	}
	for _, d := range st.Decisions {
		if d.Version != 2 || !strings.HasPrefix(d.Actor, "ui:") {
			t.Fatal("lost UI decision provenance")
		}
	}
}

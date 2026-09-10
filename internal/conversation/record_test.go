package conversation

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestIdentityVectors(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/conversation/identity-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct {
		Domain string
		Fields []string
		Key    string
	}
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatal(err)
	}
	for _, v := range vectors {
		if got := Identity(v.Domain, v.Fields...); got != v.Key {
			t.Fatalf("identity mismatch: %s", got)
		}
	}
	s := SourceKey{"fixture", 1, 0, "namespace", "stream"}
	if s.NativeKey("native") != Identity("conversation", "namespace", "native") {
		t.Fatal("native identity encoding mismatch")
	}
	changed := s
	changed.LogicalStream = "other"
	if changed.ConversationKey("native") == s.ConversationKey("native") {
		t.Fatal("logical stream collision")
	}
	for _, bad := range []SourceKey{{"", 1, 0, "ns", "s"}, {"a", 2, 0, "ns", "s"}, {"a", 1, -1, "ns", "s"}, {"a", 1, 0, "ns\x00", "s"}} {
		if bad.Validate() == nil {
			t.Fatal("invalid source accepted")
		}
	}
}
func TestNormalizeDescription(t *testing.T) {
	raw := []byte("  first  \r\nsecond\t\r\n\r\n")
	got, truncated, err := Normalize(raw)
	if err != nil || truncated || string(got) != "  first\nsecond" || Digest(raw) == Digest(got) {
		t.Fatal(string(got), truncated, err)
	}
	got, truncated, err = Normalize([]byte(strings.Repeat("a", 4095) + "界z"))
	if err != nil || !truncated || len(got) != 4095 || !utf8.Valid(got) {
		t.Fatal(len(got), truncated, err)
	}
	for _, bad := range [][]byte{{0xff}, []byte(" \r\n\t")} {
		if _, _, err := Normalize(bad); err == nil {
			t.Fatal("invalid description accepted")
		}
	}
	for _, bad := range []string{"prompt", "assistant", "message", "assistant_message", "compaction_marker", "decision"} {
		if (Description{Version: 1, ConversationID: strings.Repeat("a", 32), Kind: bad}).Validate() == nil {
			t.Fatal("ineligible kind accepted", bad)
		}
	}
}
func TestInactivityNeverEnds(t *testing.T) {
	now := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	r := Record{LastObservation: Observation{SourceTime: now}}
	if r.Lifecycle(now.Add(DefaultInactivity-time.Nanosecond)) != "started" || r.Lifecycle(now.Add(DefaultInactivity)) != "inactive-by-policy" || r.Ended != nil {
		t.Fatal("inactivity lifecycle")
	}
	r.ResumeCount = 1
	if r.Lifecycle(now) != "resumed" {
		t.Fatal("resume display")
	}
}

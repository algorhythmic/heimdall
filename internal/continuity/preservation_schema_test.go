package continuity

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestPreservationRequestStrictBoundary(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/preservation/requests.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []json.RawMessage
	if err = json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures {
		if _, err = DecodePreservation(f); err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{strings.Replace(string(f), `"version": 1`, `"version": 2`, 1), strings.Replace(string(f), `"version": 1`, `"version": 1,"version": 1`, 1), strings.Replace(string(f), `"version": 1`, `"version": 1,"actor": "cli"`, 1), strings.Replace(string(f), `"expected_task_revision": 1`, `"expected_task_revision": 0`, 1), string(f) + `{}`} {
			if _, err = DecodePreservation([]byte(bad)); err == nil {
				t.Fatal("invalid envelope accepted")
			}
		}
		var fields map[string]any
		json.Unmarshal(f, &fields)
		payload, ok := fields["observe"].(map[string]any)
		if !ok {
			payload = fields["plan"].(map[string]any)
		}
		payload["snapshot"] = map[string]string{"stage": "published"}
		bad, _ := json.Marshal(fields)
		if _, err = DecodePreservation(bad); err == nil {
			t.Fatal("caller-supplied observation accepted")
		}
	}
}

package continuity

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestProgressRequestVersionsAndForgedAuthority(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/progress/requests.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []json.RawMessage
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, raw := range fixtures {
		if _, err := DecodeProgress(raw); err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{
			strings.Replace(string(raw), `"version": 1`, `"version": 2`, 1),
			strings.Replace(string(raw), `"version": 1`, `"version": 1, "version": 1`, 1),
			strings.Replace(string(raw), `"version": 1`, `"version": 1, "actor": "cli"`, 1),
			strings.Replace(string(raw), `"expected_task_revision": 1`, `"expected_task_revision": 0`, 1),
			string(raw) + `{}`,
		} {
			if _, err := DecodeProgress([]byte(bad)); err == nil {
				t.Fatal("malformed progress request accepted", bad)
			}
		}
		var fields map[string]any
		json.Unmarshal(raw, &fields)
		payload, ok := fields["proposal"].(map[string]any)
		if !ok {
			payload = fields["review"].(map[string]any)
		}
		payload["observation"] = map[string]any{"digest": strings.Repeat("a", 64)}
		bad, _ := json.Marshal(fields)
		if _, err := DecodeProgress(bad); err == nil {
			t.Fatal("submitted observation accepted")
		}
	}
}

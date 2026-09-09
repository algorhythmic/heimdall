package continuity

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestDependencyRequestStrictSchema(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/dependencies/requests.json")
	if err != nil {
		t.Fatal(err)
	}
	var items []json.RawMessage
	if err = json.Unmarshal(raw, &items); err != nil {
		t.Fatal(err)
	}
	for _, raw := range items {
		if _, err := DecodeDependency(raw); err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{strings.Replace(string(raw), `"version": 1`, `"version": 2`, 1), strings.Replace(string(raw), `"version": 1`, `"version": 1,"version": 1`, 1), strings.Replace(string(raw), `"version": 1`, `"version": 1,"actor": "cli"`, 1), strings.Replace(string(raw), `"expected_prerequisite_revision": 1`, `"expected_prerequisite_revision": 0`, 1), strings.Replace(string(raw), `"depends_on": "beta"`, `"depends_on": "alpha"`, 1), strings.Replace(string(raw), `"depends_on": "beta"`, `"depends_on": "beta#step"`, 1), string(raw) + `{}`} {
			if _, err := DecodeDependency([]byte(bad)); err == nil {
				t.Fatal("malformed dependency accepted")
			}
		}
	}
}

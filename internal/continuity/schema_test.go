package continuity

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestRequestFixtures(t *testing.T) {
	b, err := os.ReadFile("../../testdata/continuity/requests.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []json.RawMessage
	if err = json.Unmarshal(b, &fixtures); err != nil {
		t.Fatal(err)
	}
	if len(fixtures) != 5 {
		t.Fatal("missing operation fixtures")
	}
	for _, fixture := range fixtures {
		r, err := Decode(fixture)
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(r)
		if _, err := Decode(encoded); err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{
			strings.Replace(string(fixture), `"version":1`, `"version":2`, 1),
			strings.Replace(string(fixture), `"version":1`, `"version":1,"version":1`, 1),
			strings.Replace(string(fixture), `"version":1`, `"version":1,"actor":"cli"`, 1),
			strings.Replace(string(fixture), `"expected_task_revision":1`, `"expected_task_revision":0`, 1),
		} {
			if _, err := Decode([]byte(bad)); err == nil {
				t.Fatalf("accepted malformed request: %s", bad)
			}
		}
	}
	nested := strings.Replace(string(fixtures[0]), `"previous":"none"`, `"previous":"none","previous":"none"`, 1)
	if _, err := Decode([]byte(nested)); err == nil {
		t.Fatal("duplicate nested key accepted")
	}
}

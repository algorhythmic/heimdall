package continuity

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestArtifactRequestFixturesAndVersions(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/artifacts/requests.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct{ Artifact, Checkpoint []json.RawMessage }
	if err = json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	for _, body := range f.Artifact {
		if _, err = DecodeArtifact(body); err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{
			strings.Replace(string(body), `"version": 1`, `"version": 99`, 1),
			strings.Replace(string(body), `"git": false`, `"git": false, "digest": "forged"`, 1),
			strings.Replace(string(body), `"environment": "local-test"`, `"environment": "../other"`, 1),
			strings.Replace(string(body), `"git": false`, `"git": false, "git": true`, 1),
		} {
			if _, err = DecodeArtifact([]byte(bad)); err == nil {
				t.Fatal("invalid artifact request accepted", bad)
			}
		}
	}
	for _, body := range f.Checkpoint {
		if _, err = Decode(body); err != nil {
			t.Fatal(err)
		}
		for _, version := range []string{"1", "3", "99"} {
			bad := strings.Replace(string(body), `"version": 2`, `"version": `+version, 1)
			if _, err = Decode([]byte(bad)); err == nil {
				t.Fatal("wrong checkpoint request version accepted")
			}
		}
		var r map[string]any
		json.Unmarshal(body, &r)
		cp := r["checkpoint"].(map[string]any)
		refs := cp["artifacts"].([]any)
		for _, invalid := range []any{nil, []any{}, append(refs, refs[0])} {
			cp["artifacts"] = invalid
			for _, version := range []int{1, 2} {
				r["version"] = version
				bad, _ := json.Marshal(r)
				if _, err = Decode(bad); err == nil {
					t.Fatal("invalid artifact references accepted", string(bad))
				}
			}
		}
	}
}

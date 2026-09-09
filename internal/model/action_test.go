package model

import (
	"encoding/json"
	"os"
	"testing"
)

func TestActionWireConformance(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/actions/wire-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Execution    []string         `json:"execution_states"`
		Verification []string         `json:"verification_states"`
		Reference    map[string]any   `json:"reference"`
		Reject       []map[string]any `json:"reject_patches"`
	}
	if err = StrictJSON(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Execution) != 6 || len(fixture.Verification) != 5 {
		t.Fatal("wire state list changed")
	}
	for _, s := range fixture.Execution {
		if !ValidActionExecution(s) {
			t.Fatal(s)
		}
	}
	for _, s := range fixture.Verification {
		if !ValidActionVerification(s) {
			t.Fatal(s)
		}
	}
	if ValidActionExecution("succeeded") || ValidActionVerification("succeeded") {
		t.Fatal("API success conflated with verification")
	}
	raw, _ = json.Marshal(fixture.Reference)
	var valid BrowserActionRef
	if err = StrictJSON(raw, &valid); err != nil || valid.Validate() != nil {
		t.Fatal(valid, err)
	}
	for _, patch := range fixture.Reject {
		row := Clone(fixture.Reference)
		for k, v := range patch {
			row[k] = v
		}
		raw, _ = json.Marshal(row)
		var v BrowserActionRef
		if err = StrictJSON(raw, &v); err == nil && v.Validate() == nil {
			t.Fatal("bad wire reference accepted", patch)
		}
	}
}

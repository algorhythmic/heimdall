package capture

import (
	"strings"
	"testing"
)

func TestGrammar(t *testing.T) {
	for _, s := range []string{"abc/reference: why with spaces", "^abc,def/study: 学ぶ 理由", "unassigned/candidate: new idea"} {
		if _, e := Parse(s); e != nil {
			t.Fatalf("%q: %v", s, e)
		}
	}
	for _, s := range []string{"abc/reference:no space", "abc/reference: ", "abc/reference: bad\nline", "abc,abc/task: duplicate", "abc,unassigned/task: mixed", "abc/wrong: nope", "abc/reference: " + strings.Repeat("界", 201), "abc: broken"} {
		if _, e := Parse(s); e == nil {
			t.Fatalf("accepted %q", s)
		}
	}
}

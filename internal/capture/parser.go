package capture

import (
	"fmt"
	"heimdall/internal/model"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Line struct {
	Origin  bool
	Targets []string
	Kind    string
	Why     string
}
type ParseError struct {
	Production string `json:"production"`
	Offset     int    `json:"offset"`
	Detail     string `json:"detail"`
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s at byte %d: %s", e.Production, e.Offset, e.Detail)
}
func Parse(s string) (Line, error) {
	var l Line
	offset := 0
	fail := func(p string, n int, msg string) (Line, error) { return l, &ParseError{p, n, msg} }
	if !utf8.ValidString(s) {
		return fail("why", 0, "invalid UTF-8")
	}
	if strings.HasPrefix(s, "^") {
		l.Origin = true
		offset = 1
	}
	slash := strings.Index(s[offset:], "/")
	if slash < 0 {
		return fail("streams", offset, "missing /")
	}
	slash += offset
	l.Targets = strings.Split(s[offset:slash], ",")
	seen := map[string]bool{}
	pos := offset
	for _, id := range l.Targets {
		if (id != "unassigned" && !model.ValidID(id)) || seen[id] {
			return fail("stream", pos, "invalid or duplicate stream")
		}
		seen[id] = true
		pos += len(id) + 1
	}
	if seen["unassigned"] && len(l.Targets) > 1 {
		return fail("streams", offset, "unassigned cannot be combined")
	}
	colon := strings.Index(s[slash+1:], ":")
	if colon < 0 {
		return fail("kind", slash+1, "missing :")
	}
	colon += slash + 1
	l.Kind = s[slash+1 : colon]
	if !model.Contains([]string{"candidate", "reference", "task", "study"}, l.Kind) {
		return fail("kind", slash+1, "unknown kind")
	}
	if colon+1 >= len(s) || s[colon+1] != ' ' {
		return fail("line", colon+1, "expected space after :")
	}
	raw := s[colon+2:]
	for i, r := range raw {
		if unicode.IsControl(r) {
			return fail("why", colon+2+i, "control character")
		}
	}
	l.Why = strings.TrimSpace(raw)
	if n := utf8.RuneCountInString(l.Why); n < 1 || n > 200 {
		return fail("why", colon+2, "expected 1..200 Unicode characters")
	}
	return l, nil
}

package surface

import (
	"strings"
	"testing"
)

// Fixed vectors are calculated independently from the r4 UTF-8 hash formula.
func TestIdentityVectors(t *testing.T) {
	for pointer, want := range map[string]string{
		"https://example.com/page?a=1":     "a2a1afc22730",
		"https://claude.ai/chat/session-1": "39f032e04033",
		"herdr:workspace-1":                "9ed456b050f1",
		"/work/project/.git":               "95fb470b063d",
	} {
		got, err := Identify(pointer)
		if err != nil || got.ID != want {
			t.Fatalf("%q: got %+v, %v; want %s", pointer, got, err, want)
		}
	}
}

func TestIdentifyKindsAndNormalization(t *testing.T) {
	digest := strings.Repeat("a", 64)
	for _, tt := range []struct{ pointer, kind, normalized string }{
		{"https://EXAMPLE.com/page/?utm_source=mail&a=1&fbclid=x&gclid=y#heading", "tab", "https://example.com/page?a=1"},
		{"https://claude.ai/chat/session-1/?utm_campaign=x#last", "chat", "https://claude.ai/chat/session-1"},
		{"https://chatgpt.com/c/session-2", "chat", "https://chatgpt.com/c/session-2"},
		{"https://claude.ai/chat/", "tab", "https://claude.ai/chat"},
		{"https://claude.ai.evil.test/chat/session-1", "tab", "https://claude.ai.evil.test/chat/session-1"},
		{"https://example.com/", "tab", "https://example.com"},
		{"https://example.com/a%2F#fragment", "tab", "https://example.com/a%2F"},
		{"https://example.com/a/?q=a%20b&q=a+b&%75tm_source=x&b=2", "tab", "https://example.com/a?q=a%20b&q=a+b&b=2"},
		{"https://example.com/?utm_source=x", "tab", "https://example.com"},
		{"https://example.com/?UTM_source=x", "tab", "https://example.com?UTM_source=x"},
		{"about:blank#fragment", "tab", "about:blank"},
		{"file:///tmp/document/", "tab", "file:///tmp/document"},
		{"artifact:" + digest, "artifact", "artifact:" + digest},
		{"herdr:workspace-1", "terminal", "herdr:workspace-1"},
		{"maildir:<id?part#one@example.com>", "mail", "maildir:<id?part#one@example.com>"},
		{"imap:<id@example.com>", "mail", "imap:<id@example.com>"},
		{"/work/project/.git/", "repo", "/work/project/.git"},
		{"/work/project/.git/config", "repo", "/work/project/.git/config"},
		{"/work/日本語/.git", "repo", "/work/日本語/.git"},
	} {
		t.Run(tt.pointer, func(t *testing.T) {
			got, err := Identify(tt.pointer)
			if err != nil {
				t.Fatal(err)
			}
			if got.Kind != tt.kind || got.NormalizedPointer != tt.normalized || got.Pointer != tt.pointer || len(got.ID) != 12 {
				t.Fatalf("unexpected identity: %+v", got)
			}
			again, err := Identify(got.NormalizedPointer)
			if err != nil || again.ID != got.ID || again.NormalizedPointer != got.NormalizedPointer {
				t.Fatalf("normalization is not idempotent: %+v, %v", again, err)
			}
		})
	}
}

func TestIdentifyRejectsMalformedPointers(t *testing.T) {
	for _, pointer := range []string{
		"", " relative/path", "relative/path", "/work/project", "/work/.github/config",
		"https://example.com\n", "https://example.com/\x00", string([]byte{0xff}),
		"https://", "https:example.com", "https://example.com/%zz",
		"https://example.com/?x=%zz", "https://example.com/?%zz=x",
		"https://name:secret@example.com/", "herdr:", "maildir:", "imap:",
		"artifact:", "artifact:abc", "artifact:" + strings.Repeat("A", 64),
		"artifact:" + strings.Repeat("z", 64),
	} {
		t.Run(pointer, func(t *testing.T) {
			got, err := Identify(pointer)
			if err == nil || got != (Identity{}) {
				t.Fatalf("malformed pointer accepted: %+v, %v", got, err)
			}
			if strings.Contains(err.Error(), pointer) && pointer != "" {
				t.Fatalf("error discloses source pointer: %v", err)
			}
		})
	}
}

func TestIdentifyPreservesMeaningfulDistinctions(t *testing.T) {
	for _, pair := range [][2]string{
		{"https://example.com/?a=1", "https://example.com/?a=2"},
		{"https://example.com/?a=1&a=2", "https://example.com/?a=2&a=1"},
		{"https://example.com/path%2F", "https://example.com/path/"},
		{"https://example.com/Path", "https://example.com/path"},
		{"https://example.com/", "http://example.com/"},
		{"herdr:workspace-1", "herdr:workspace-2"},
		{"maildir:message-1", "imap:message-1"},
		{"/work/a/.git", "/work/b/.git"},
	} {
		a, err := Identify(pair[0])
		if err != nil {
			t.Fatal(err)
		}
		b, err := Identify(pair[1])
		if err != nil {
			t.Fatal(err)
		}
		if a.ID == b.ID {
			t.Fatalf("distinct pointers conflated: %q, %q", pair[0], pair[1])
		}
	}
}

func FuzzIdentifyNormalization(f *testing.F) {
	for _, seed := range []string{"https://example.com/a/?utm_source=x#f", "herdr:w1", "/work/.git", "about:blank"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, pointer string) {
		got, err := Identify(pointer)
		if err != nil {
			return
		}
		again, err := Identify(got.NormalizedPointer)
		if err != nil || again.ID != got.ID || again.NormalizedPointer != got.NormalizedPointer {
			t.Fatalf("unstable normalization: %+v -> %+v, %v", got, again, err)
		}
	})
}

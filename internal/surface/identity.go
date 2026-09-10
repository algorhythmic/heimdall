// Package surface identifies observed content independently of task ownership
// and runtime containers. Identification does not observe the filesystem or
// confer authority to read or act on a surface.
package surface

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Identity follows handoff r4 §5.3. Pointer retains the source's exact locator;
// NormalizedPointer alone participates in content identity. Container epochs,
// task bindings and observations belong to the consuming sensor records.
type Identity struct {
	ID                string `json:"id"`
	Kind              string `json:"kind"`
	Pointer           string `json:"pointer"`
	NormalizedPointer string `json:"normalized_pointer"`
}

// Identify derives an observed-surface identity without I/O. Unsupported or
// malformed locators return an error so a sensor can report a coverage gap.
// Callers must still apply their source's collection and retention policy.
func Identify(pointer string) (Identity, error) {
	if pointer == "" || !utf8.ValidString(pointer) || strings.TrimSpace(pointer) != pointer || strings.ContainsFunc(pointer, unicode.IsControl) {
		return Identity{}, fmt.Errorf("invalid observed surface pointer")
	}
	kind, normalized, err := normalize(pointer)
	if err != nil {
		return Identity{}, err
	}
	digest := sha256.Sum256([]byte(kind + "|" + normalized))
	return Identity{
		ID: hex.EncodeToString(digest[:6]), Kind: kind,
		Pointer: pointer, NormalizedPointer: normalized,
	}, nil
}

func normalize(pointer string) (string, string, error) {
	for _, spec := range []struct{ prefix, kind string }{
		{"artifact:", "artifact"}, {"herdr:", "terminal"},
		{"maildir:", "mail"}, {"imap:", "mail"},
	} {
		if !strings.HasPrefix(pointer, spec.prefix) {
			continue
		}
		value := strings.TrimPrefix(pointer, spec.prefix)
		if value == "" || strings.TrimSpace(value) != value {
			return "", "", fmt.Errorf("empty or malformed observed surface identifier")
		}
		if spec.kind == "artifact" {
			decoded, err := hex.DecodeString(value)
			if err != nil || len(decoded) != sha256.Size || strings.ToLower(value) != value {
				return "", "", fmt.Errorf("artifact surface requires a lowercase sha256 content digest")
			}
		}
		// These are opaque native identifiers, not URLs. In particular a mail
		// message-id can contain URL punctuation that must not be stripped.
		return spec.kind, pointer, nil
	}
	if strings.HasPrefix(pointer, "/") {
		for _, component := range strings.Split(pointer, "/") {
			if component == ".git" {
				// Do not clean dot segments or resolve symlinks: that would
				// invent filesystem equivalence without a source observation.
				return "repo", strings.TrimRight(pointer, "/"), nil
			}
		}
		return "", "", fmt.Errorf("repository pointer requires an absolute path containing a .git component")
	}
	u, err := url.Parse(pointer)
	if err != nil || u.Scheme == "" || (u.Host == "" && u.Opaque == "" && u.Path == "") {
		return "", "", fmt.Errorf("invalid observed surface URL")
	}
	if u.User != nil {
		return "", "", fmt.Errorf("observed surface URL must not contain credentials")
	}
	// HTTP(S) locators need a host; opaque browser URLs such as about:blank
	// remain identifiable when a configured sensor supports observing them.
	u.Scheme = strings.ToLower(u.Scheme)
	if (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() == "" {
		return "", "", fmt.Errorf("observed HTTP surface requires a host")
	}
	u.Host = strings.ToLower(u.Host)
	u.Fragment, u.RawFragment = "", ""
	// Work on the escaped path so a terminal %2F is never mistaken for a
	// literal slash. Keep other escaping exactly as supplied by the source.
	escaped := strings.TrimRight(u.EscapedPath(), "/")
	if escaped == "" && u.Host == "" && u.Opaque == "" {
		// A hostless URL's root is its locator (file:///); removing it
		// would leave only a scheme and make normalization non-repeatable.
		escaped = "/"
	}
	u.Path, err = url.PathUnescape(escaped)
	if err != nil {
		return "", "", fmt.Errorf("invalid observed surface URL path")
	}
	u.RawPath = escaped
	query, err := stripTracking(u.RawQuery)
	if err != nil {
		return "", "", err
	}
	u.RawQuery = query
	if query == "" {
		u.ForceQuery = false
	}
	kind := "tab"
	if (u.Scheme == "https" || u.Scheme == "http") &&
		((u.Host == "claude.ai" && chatPath(u.EscapedPath(), "/chat/")) ||
			(u.Host == "chatgpt.com" && chatPath(u.EscapedPath(), "/c/"))) {
		kind = "chat"
	}
	return kind, u.String(), nil
}

func chatPath(path, prefix string) bool {
	return strings.HasPrefix(path, prefix) && len(path) > len(prefix)
}

func stripTracking(query string) (string, error) {
	if query == "" {
		return "", nil
	}
	kept := make([]string, 0)
	for _, part := range strings.Split(query, "&") {
		key, value, _ := strings.Cut(part, "=")
		key, err := url.QueryUnescape(key)
		if err != nil {
			return "", fmt.Errorf("invalid observed surface URL query")
		}
		if _, err := url.QueryUnescape(value); err != nil {
			return "", fmt.Errorf("invalid observed surface URL query")
		}
		if strings.HasPrefix(key, "utm_") || key == "fbclid" || key == "gclid" {
			continue
		}
		kept = append(kept, part)
	}
	// Preserve order, repetitions and escaping of meaningful parameters.
	return strings.Join(kept, "&"), nil
}

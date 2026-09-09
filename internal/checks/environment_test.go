package checks

import (
	"os"
	"strings"
	"testing"
)

func TestEvaluatorEnvironmentWhitelist(t *testing.T) {
	for _, key := range []string{"HOME", "XDG_CACHE_HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "GOCACHE", "GOPATH", "GOMODCACHE", "GOFLAGS", "LANG", "LC_ALL", "TMPDIR"} {
		t.Setenv(key, "allowed")
	}
	t.Setenv("HEIMDALL_TEST_SECRET", "excluded")
	t.Setenv("HEIMDALL_NAMED_INPUT", "selected")
	env := minimalEnv("HEIMDALL_NAMED_INPUT", "HOME")
	values := map[string]string{}
	for _, entry := range env {
		k, v, _ := strings.Cut(entry, "=")
		if _, exists := values[k]; exists {
			t.Fatal("duplicate", k)
		}
		values[k] = v
	}
	if values["HEIMDALL_TEST_SECRET"] != "" || values["HEIMDALL_NAMED_INPUT"] != "selected" {
		t.Fatal("environment boundary failed")
	}
	for _, key := range []string{"HOME", "XDG_CACHE_HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "GOCACHE", "GOPATH", "GOMODCACHE", "GOFLAGS", "LANG", "LC_ALL", "TMPDIR"} {
		if values[key] != os.Getenv(key) {
			t.Fatal("missing", key)
		}
	}
}

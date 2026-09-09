package model

import "testing"

func TestDeclaredSessionPathsAndBounds(t *testing.T) {
	l := SessionLocator{Adapter: "generic", Environment: "local", Host: "host", SourceEpoch: "epoch", SessionID: "session", WorkspaceID: "workspace", PaneID: "pane", Platform: "linux", Cwd: "/missing/path"}
	for _, tt := range []struct {
		platform, path string
		valid          bool
	}{
		{"linux", "/missing/path", true}, {"darwin", "/Users/synthetic/work", true}, {"windows", `C:\synthetic\work`, true}, {"windows", `\\server\share\work`, true}, {"windows", `C:/synthetic/work`, true},
		{"linux", "relative/path", false}, {"windows", `C:relative`, false}, {"windows", `\relative`, false}, {"windows", `\\?\C:\synthetic`, false}, {"linux", "/bad\x1b/path", false},
	} {
		t.Run(tt.platform+tt.path, func(t *testing.T) {
			copy := l
			copy.Platform = tt.platform
			copy.Cwd = tt.path
			if err := copy.Validate(); (err == nil) != tt.valid {
				t.Fatal(err)
			}
		})
	}
	for _, mutate := range []func(*SessionLocator){
		func(l *SessionLocator) { l.Adapter = "herdr" }, func(l *SessionLocator) { l.SourceEpoch = "" }, func(l *SessionLocator) { l.PaneID = "" }, func(l *SessionLocator) { l.Repository = "/repo" }, func(l *SessionLocator) { l.AgentSessionID = "bad\nagent" },
	} {
		copy := l
		mutate(&copy)
		if err := copy.Validate(); err == nil {
			t.Fatal("invalid locator accepted", copy)
		}
	}
}

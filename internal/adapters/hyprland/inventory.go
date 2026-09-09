package hyprland

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/model"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

var addressPattern = regexp.MustCompile(`^0x[0-9a-f]{1,16}$`)

func display(s string) string {
	var out strings.Builder
	out.Grow(512)
	for _, r := range s {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			r = ' '
		}
		if out.Len()+utf8.RuneLen(r) > 512 {
			break
		}
		out.WriteRune(r)
	}
	return out.String()
}

// Native JSON is extensible. Required identity and topology are checked after
// decoding; unknown compositor fields do not become owned Heimdall metadata.
func inventory(ctx context.Context, c Connection) (model.DesktopSnapshot, error) {
	v := model.DesktopSnapshot{Version: 1, SourceEpoch: c.Source().Epoch, Host: c.Source().Host, CompositorVersion: "0.56.2", Windows: []model.DesktopWindow{}, Workspaces: []model.DesktopWorkspace{}, Monitors: []model.DesktopMonitor{}}
	read := func(command string, dst any) error {
		b, err := c.Read(ctx, command)
		if err != nil {
			return err
		}
		if err = json.Unmarshal(b, dst); err != nil {
			return fmt.Errorf("invalid_%s_json", command)
		}
		return nil
	}
	var clients []struct {
		Address      string
		StableID     string `json:"stableId"`
		PID          int
		Class, Title string
		Workspace    struct {
			ID   int
			Name string
		}
		Monitor                          int
		At, Size                         []int
		Floating, Pinned, Hidden, Mapped bool
		Fullscreen                       int
	}
	var workspaces []struct {
		ID            int
		Name, Monitor string
		MonitorID     int
	}
	var monitors []struct {
		ID                                int
		Name                              string
		X, Y, Width, Height, Transform    int
		Scale                             float64
		ActiveWorkspace, SpecialWorkspace struct{ ID int }
	}
	if err := read("clients", &clients); err != nil {
		return v, err
	}
	if err := read("workspaces", &workspaces); err != nil {
		return v, err
	}
	if err := read("monitors", &monitors); err != nil {
		return v, err
	}
	if clients == nil || workspaces == nil || monitors == nil || len(clients) > 256 || len(workspaces) > 128 || len(monitors) > 32 || len(monitors) == 0 {
		return v, fmt.Errorf("empty_or_oversize_inventory")
	}
	ms := map[int]string{}
	mn := map[string]bool{}
	for _, m := range monitors {
		if _, ok := ms[m.ID]; ok || m.ID < 0 || m.Name == "" || len(m.Name) > 256 || mn[m.Name] || m.Width <= 0 || m.Height <= 0 || m.Scale <= 0 || m.Scale > 16 || m.Transform < 0 || m.Transform > 7 {
			return v, fmt.Errorf("invalid_monitor_topology")
		}
		ms[m.ID] = m.Name
		mn[m.Name] = true
		v.Monitors = append(v.Monitors, model.DesktopMonitor{ID: m.ID, Name: m.Name, X: m.X, Y: m.Y, Width: m.Width, Height: m.Height, Scale: m.Scale, Transform: m.Transform, ActiveWorkspace: m.ActiveWorkspace.ID, SpecialWorkspace: m.SpecialWorkspace.ID})
	}
	ws := map[int]string{}
	for _, w := range workspaces {
		if _, ok := ws[w.ID]; ok || w.ID == 0 || w.Name == "" || len(w.Name) > 256 || ms[w.MonitorID] != w.Monitor {
			return v, fmt.Errorf("inconsistent_workspace_topology")
		}
		ws[w.ID] = w.Name
		v.Workspaces = append(v.Workspaces, model.DesktopWorkspace{ID: w.ID, Name: w.Name, MonitorID: w.MonitorID, MonitorName: w.Monitor})
	}
	ids := map[string]bool{}
	addresses := map[string]bool{}
	for _, w := range clients {
		if !w.Mapped {
			continue
		}
		identity := model.WindowIdentity{SourceEpoch: v.SourceEpoch, StableID: w.StableID}
		if identity.Validate() != nil || ids[w.StableID] || addresses[w.Address] || !addressPattern.MatchString(w.Address) || w.PID < 1 || len(w.At) != 2 || len(w.Size) != 2 || w.Size[0] < 0 || w.Size[1] < 0 || ws[w.Workspace.ID] != w.Workspace.Name || w.Workspace.Name == "" || ms[w.Monitor] == "" {
			return v, fmt.Errorf("ambiguous_or_inconsistent_window")
		}
		ids[w.StableID] = true
		addresses[w.Address] = true
		v.Windows = append(v.Windows, model.DesktopWindow{Identity: identity, Address: w.Address, PID: w.PID, Class: display(w.Class), Title: display(w.Title), WorkspaceID: w.Workspace.ID, MonitorID: w.Monitor, At: [2]int(w.At), Size: [2]int(w.Size), Floating: w.Floating, Pinned: w.Pinned, Hidden: w.Hidden, Fullscreen: w.Fullscreen})
	}
	sort.Slice(v.Monitors, func(i, j int) bool { return v.Monitors[i].ID < v.Monitors[j].ID })
	sort.Slice(v.Workspaces, func(i, j int) bool { return v.Workspaces[i].ID < v.Workspaces[j].ID })
	sort.Slice(v.Windows, func(i, j int) bool { return v.Windows[i].Identity.StableID < v.Windows[j].Identity.StableID })
	b, err := json.Marshal(v)
	if err != nil || len(b) > 384<<10 {
		return v, fmt.Errorf("inventory_response_limit")
	}
	v.ID = hash(string(b))
	return v, nil
}

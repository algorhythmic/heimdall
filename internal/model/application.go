package model

import (
	"fmt"
	"strconv"
	"time"
)

// ApplicationRecipe is a local, explicitly accepted launch declaration. It
// cannot be populated from captured process arguments, titles or window class.
type ApplicationRecipe struct {
	Version      int              `json:"version"`
	ID           string           `json:"id"`
	Target       string           `json:"target"`
	TaskRevision int64            `json:"task_revision"`
	ManifestID   string           `json:"manifest_id"`
	SurfaceID    string           `json:"surface_id"`
	Previous     string           `json:"previous"`
	Active       bool             `json:"active"`
	Spec         *ApplicationSpec `json:"spec,omitempty"`
	Actor        string           `json:"actor"`
	At           time.Time        `json:"at"`
}

type ApplicationSpec struct {
	Browser          *BrowserRestore `json:"browser,omitempty"`
	Editor           *EditorRestore  `json:"editor,omitempty"`
	SessionBindingID string          `json:"session_binding_id,omitempty"`
	Adapter          string          `json:"adapter"`
	Executable       string          `json:"executable,omitempty"`
	ExecutableDigest string          `json:"executable_digest,omitempty"`
	Command          string          `json:"command,omitempty"`
	CommandDigest    string          `json:"command_digest,omitempty"`
	Argv             []string        `json:"argv"`
	Cwd              string          `json:"cwd,omitempty"`
	ClosePolicy      string          `json:"close_policy"`
}

func (s ApplicationSpec) Validate() error {
	if s.Adapter == "browser" {
		if s.Browser == nil || !OpaqueID.MatchString(s.Browser.Profile) || !BrowserURL(s.Browser.URL) || !Contains([]string{"leave_open", "owned_tab"}, s.ClosePolicy) || s.Editor != nil || s.SessionBindingID != "" || s.Executable != "" || s.ExecutableDigest != "" || s.Command != "" || s.CommandDigest != "" || s.Cwd != "" || len(s.Argv) != 0 {
			return fmt.Errorf("browser recipe requires paired profile, URL and explicit owned-tab policy")
		}
		return nil
	}
	if s.Browser != nil {
		return fmt.Errorf("browser inputs on native adapter")
	}
	if !Contains([]string{"foot", "foot-herdr", "foot-nvim"}, s.Adapter) || !declaredPath("linux", s.Executable) || !declaredPath("linux", s.Command) || !declaredPath("linux", s.Cwd) || !TokenHashPattern.MatchString(s.ExecutableDigest) || !TokenHashPattern.MatchString(s.CommandDigest) || s.Argv == nil || len(s.Argv) > 64 || !Contains([]string{"leave_open", "graceful_session_end", "detach"}, s.ClosePolicy) {
		return fmt.Errorf("reviewed foot executable, command digests, absolute cwd, argv and explicit close policy required")
	}
	if s.Adapter == "foot-herdr" {
		if !OpaqueID.MatchString(s.SessionBindingID) || len(s.Argv) != 0 || !Contains([]string{"leave_open", "detach"}, s.ClosePolicy) {
			return fmt.Errorf("Herdr requires an exact session binding, generated attach arguments and detach policy")
		}
	} else if s.SessionBindingID != "" || s.ClosePolicy == "detach" {
		return fmt.Errorf("generic terminal cannot claim session detach")
	}
	if s.Adapter == "foot-nvim" {
		if s.Editor == nil || s.Editor.Validate() != nil || len(s.Argv) != 0 || s.ClosePolicy != "leave_open" {
			return fmt.Errorf("Neovim requires structured saved-file state and leave-open policy")
		}
	} else if s.Editor != nil {
		return fmt.Errorf("editor state on non-editor adapter")
	}
	n := 0
	for _, arg := range s.Argv {
		n += len(arg)
		if len(arg) > 4096 || (arg != "" && !workspaceText(arg, 4096)) {
			return fmt.Errorf("invalid application argument")
		}
	}
	if n > 16384 {
		return fmt.Errorf("application argv exceeds bound")
	}
	return nil
}

type BrowserRestore struct {
	Profile string `json:"profile"`
	URL     string `json:"url"`
}

type BrowserWorkspaceAction struct {
	OperationID      string `json:"operation_id"`
	RecipeID         string `json:"recipe_id"`
	PreviousViewport string `json:"previous_viewport"`
}

func ApplicationSummary(r ApplicationRecipe) string {
	if r.Spec == nil {
		return "Retired recipe " + r.ID
	}
	s := r.Spec
	if s.Browser != nil {
		return fmt.Sprintf("%s · profile %s · %s · close %s", s.Adapter, s.Browser.Profile, s.Browser.URL, s.ClosePolicy)
	}
	if s.Editor != nil {
		return fmt.Sprintf("%s · %q · active file %d at %d:%d · close %s", s.Adapter, s.Editor.Files, s.Editor.Active+1, s.Editor.Line, s.Editor.Column, s.ClosePolicy)
	}
	if s.SessionBindingID != "" {
		return fmt.Sprintf("%s · binding %s · cwd %s · close %s", s.Adapter, s.SessionBindingID, s.Cwd, s.ClosePolicy)
	}
	return fmt.Sprintf("%s · %s %q · cwd %s · close %s", s.Adapter, s.Command, s.Argv, s.Cwd, s.ClosePolicy)
}

// Supported editor state contains paths and cursor position, never executable
// session scripts, terminal buffers or unsaved buffer contents.
type EditorRestore struct {
	Files  []string `json:"files"`
	Active int      `json:"active"`
	Line   int      `json:"line"`
	Column int      `json:"column"`
}

func (e EditorRestore) Validate() error {
	if len(e.Files) < 1 || len(e.Files) > 32 || e.Active < 0 || e.Active >= len(e.Files) || e.Line < 1 || e.Column < 1 || e.Line > 10000000 || e.Column > 10000000 {
		return fmt.Errorf("bounded saved-file editor state required")
	}
	seen := map[string]bool{}
	for _, path := range e.Files {
		if !declaredPath("linux", path) || seen[path] {
			return fmt.Errorf("unique absolute editor paths required")
		}
		seen[path] = true
	}
	return nil
}

func (v ApplicationRecipe) Validate() error {
	if err := ValidWorkspaceRecord(v.Version, v.ID, v.Target, v.Previous, v.Actor, v.At, v.TaskRevision); err != nil {
		return err
	}
	if !OpaqueID.MatchString(v.ManifestID) || !OpaqueID.MatchString(v.SurfaceID) {
		return fmt.Errorf("application recipe requires manifest and surface")
	}
	if v.Active {
		if v.Spec == nil {
			return fmt.Errorf("active recipe requires specification")
		}
		return v.Spec.Validate()
	}
	if v.Spec != nil {
		return fmt.Errorf("retired recipe cannot grant launch inputs")
	}
	return nil
}

func ApplicationRecipeCurrent(st State, id, target, surface string) bool {
	r := st.ApplicationRecipes[id]
	if !r.Active || r.Target != target || r.SurfaceID != surface || st.ApplicationHeads[surface] != id || st.WorkspaceHeads[target] != r.ManifestID || st.Tasks[target].Revision != r.TaskRevision || r.Spec == nil {
		return false
	}
	return ApplicationSessionCurrent(st, r)
}

func ApplicationSessionCurrent(st State, r ApplicationRecipe) bool {
	if r.Spec == nil {
		return false
	}
	b := st.SessionBindings[st.SessionHeads[r.SurfaceID]]
	if r.Spec.SessionBindingID == "" {
		return !b.Active
	}
	return b.Active && b.ID == r.Spec.SessionBindingID && b.Version == 2 && b.Target == r.Target && b.SurfaceID == r.SurfaceID && b.ManifestID == r.ManifestID && b.TaskRevision == r.TaskRevision && b.Herdr != nil && b.Locator != nil && b.Locator.Cwd == r.Spec.Cwd
}

type ApplicationLaunch struct {
	Editor           bool   `json:"editor,omitempty"`
	SessionBindingID string `json:"session_binding_id,omitempty"`
	RecipeID         string `json:"recipe_id"`
	PreviousViewport string `json:"previous_viewport"`
}
type ApplicationProcess struct {
	PID   int    `json:"pid"`
	Start string `json:"start"`
}

func (p ApplicationProcess) Valid() bool {
	n, err := strconv.ParseUint(p.Start, 10, 64)
	return p.PID > 0 && err == nil && n > 0 && strconv.FormatUint(n, 10) == p.Start
}
func ApplicationClass(attempt string) string { return "heimdall-" + attempt }

func ApplicationSessionDigest(b SessionBinding) string {
	return ContentDigest(struct {
		Locator *SessionLocator
		Herdr   *HerdrIdentity
	}{b.Locator, b.Herdr})
}

// Cancellation stops new input, not observation of an already dispatched child.
func ApplicationAssociationCurrent(st State, a ActionIntent) bool {
	if a.Native == nil || a.Native.Launch == nil {
		return false
	}
	op := st.WorkspaceOperations[a.Native.OperationID]
	copy := st
	copy.WorkspaceOperations = make(map[string]WorkspaceOperation, len(st.WorkspaceOperations))
	for id, v := range st.WorkspaceOperations {
		copy.WorkspaceOperations[id] = v
	}
	op.CancelRequested = false
	copy.WorkspaceOperations[op.Intent.ID] = op
	return ActionInputsCurrent(copy, a)
}

// A launch changes the viewport head by design. Only bindings proven by this
// operation's own attempts may be substituted when checking its original pins.
func OperationInputDigest(st State, op WorkspaceOperation, target string) string {
	copy := st
	copy.ViewportHeads = make(map[string]string, len(st.ViewportHeads))
	for k, v := range st.ViewportHeads {
		copy.ViewportHeads[k] = v
	}
	for _, id := range op.ActionIDs {
		a := st.Actions[id]
		if a.Intent.Target == target && a.Intent.Workspace != nil && a.Intent.Browser != nil && a.Intent.Browser.Pairing != nil {
			b := st.ViewportBindings[copy.ViewportHeads[a.Intent.SurfaceID]]
			proof := st.BrowserAssociations[b.BrowserAssociationID]
			if b.Active && proof.ActionRef.ID == id {
				copy.ViewportHeads[a.Intent.SurfaceID] = a.Intent.Workspace.PreviousViewport
			}
		}
		if a.Intent.Target != target || a.Intent.Native == nil || a.Intent.Native.Launch == nil {
			continue
		}
		b := st.ViewportBindings[copy.ViewportHeads[a.Intent.SurfaceID]]
		if b.ApplicationActionID == id && b.Active {
			copy.ViewportHeads[a.Intent.SurfaceID] = a.Intent.Native.Launch.PreviousViewport
		}
	}
	return SnapshotInputDigest(copy, target)
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/store"
	"io"
	"net/url"
	"strings"
	"time"
)

func conversationsCLI(ctx context.Context, o options, args []string, out io.Writer) error {
	if len(args) > 1 || (len(args) == 1 && strings.HasPrefix(args[0], "-")) {
		return fmt.Errorf("conversations [TARGET] [--json]")
	}
	path := "/conversations"
	if len(args) == 1 {
		path += "?target=" + url.QueryEscape(args[0])
	}
	raw, err := call(ctx, o, "GET", path, nil)
	if err != nil {
		return err
	}
	var views []store.ConversationView
	if err := json.Unmarshal(raw, &views); err != nil {
		return err
	}
	if o.json {
		// JSON is also commonly printed to terminals. Preserve digest metadata while
		// explicitly escaping display text, including bidi and format characters.
		for i := range views {
			views[i].Description.Text = terminalText(views[i].Description.Text)
		}
		return json.NewEncoder(out).Encode(views)
	}
	return renderConversations(out, views)
}
func renderConversations(out io.Writer, views []store.ConversationView) error {
	var b strings.Builder
	if len(views) == 0 {
		b.WriteString("No conversations.\n")
	}
	for _, v := range views {
		line := func(label, value string) { fmt.Fprintf(&b, "%s%s\n", label, terminalText(value)) }
		line("", v.ID+"  "+v.Kind+"  "+v.Lifecycle)
		if v.Task != nil {
			line("  Task: ", fmt.Sprintf("%s (revision %d)", v.Task.Target, v.Task.Revision))
		} else {
			b.WriteString("  Task: unbound\n")
		}
		line("  Started: ", v.StartedAt.UTC().Format(time.RFC3339))
		if v.ResumeCount > 0 {
			line("  Resumed: ", fmt.Sprintf("%d; last %s", v.ResumeCount, v.LastStartedAt.UTC().Format(time.RFC3339)))
		}
		if v.Ended != nil {
			line("  Ended: ", v.Ended.EndedAt.UTC().Format(time.RFC3339))
		}
		if v.TranscriptRef != nil {
			line("  Transcript: ", v.TranscriptRef.Locator+" ("+v.TranscriptRef.Digest+")")
		} else {
			line("  Transcript gap: ", v.TranscriptGap)
		}
		d := v.Description
		line("  Description: ", strings.TrimSpace(d.Availability+" "+d.Kind+" "+d.Digest))
		if d.Gap != "" {
			line("  Evidence gap: ", d.Gap)
		} else {
			line("  ", d.Text)
		}
		if d.Truncated {
			b.WriteString("  Retention: truncated at 4096 UTF-8 bytes\n")
		}
	}
	_, err := io.WriteString(out, b.String())
	return err
}

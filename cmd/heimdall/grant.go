package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"heimdall/internal/authz"
	"heimdall/internal/client"
	"heimdall/internal/continuity"
	"heimdall/internal/model"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type clientCredential = client.Credential

var readCredential = client.ReadCredential

func grantCLI(ctx context.Context, o options, verb string, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("grant issue|activate|list|revoke or client task|history|context required")
	}
	action := args[0]
	print := func(b []byte, err error) error {
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, string(b))
		return err
	}
	if verb == "grant" && action == "list" {
		if len(args) != 1 {
			return fmt.Errorf("unexpected arguments")
		}
		return print(call(ctx, o, "GET", "/grants", nil))
	}
	if verb == "grant" && action == "revoke" {
		if len(args) != 2 || !model.OpaqueID.MatchString(args[1]) {
			return fmt.Errorf("grant revoke ID required")
		}
		id := o.requestID
		if id == "" {
			id = model.NewID()
		}
		return print(call(ctx, o, "POST", "/grants/command", authz.Request{Version: 1, ID: id, Op: "grant.revoke", GrantID: args[1]}))
	}
	f := flag.NewFlagSet(verb+" "+action, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	credential := f.String("credential", "", "private credential file")
	if verb == "grant" && action == "activate" {
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if f.NArg() != 0 || *credential == "" {
			return fmt.Errorf("grant activate --credential FILE required")
		}
		c, err := readCredential(*credential)
		if err != nil {
			return err
		}
		o.dir = c.DataDir
		return print(call(ctx, o, "POST", "/grants/command", c.Issue))
	}
	if len(args) < 2 {
		return fmt.Errorf("target required")
	}
	target := args[1]
	if verb == "grant" && action == "issue" {
		name := f.String("name", "", "client label")
		expires := f.String("expires", "", "required RFC3339 expiry, at most 30 days")
		subtree := f.Bool("subtree", false, "include descendants")
		checkpointWrite := f.Bool("checkpoint-write", false, "allow checkpoint progress writes")
		resources := f.String("resources", "", "comma-separated resource IDs allowed for live observations")
		output := f.String("output", "", "new private credential file")
		if err := f.Parse(args[2:]); err != nil {
			return err
		}
		if f.NArg() != 0 || *output == "" || *credential != "" {
			return fmt.Errorf("grant issue TARGET --name NAME --expires TIME --output FILE required")
		}
		expiry, err := time.Parse(time.RFC3339, *expires)
		if err != nil {
			return err
		}
		id := o.requestID
		if id == "" {
			id = model.NewID()
		}
		ids := []string{}
		if *resources != "" {
			ids = strings.Split(*resources, ",")
		}
		sort.Strings(ids)
		secret := make([]byte, 32)
		if _, err = rand.Read(secret); err != nil {
			return err
		}
		token := hex.EncodeToString(secret)
		g := model.Grant{Version: 1, ID: id, Name: *name, Target: target, Subtree: *subtree, ResourceIDs: ids, TokenHash: authz.HashToken(token), ExpiresAt: expiry}
		if *checkpointWrite {
			g.Version = 2
			g.CheckpointWrite = true
		}
		check := g
		check.At = time.Now().UTC()
		check.Actor = "cli"
		if err = check.Validate(); err != nil {
			return err
		}
		c := clientCredential{Version: 1, DataDir: o.dir, Token: token, Issue: authz.Request{Version: 1, ID: id, Op: "grant.issue", Grant: &authz.IssueInput{Name: g.Name, Target: g.Target, Subtree: g.Subtree, ResourceIDs: g.ResourceIDs, TokenHash: g.TokenHash, ExpiresAt: g.ExpiresAt}}}
		c.Issue.Grant.CheckpointWrite = g.CheckpointWrite
		path, err := filepath.Abs(*output)
		if err != nil {
			return err
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("create credential file (use grant activate for a saved request): %w", err)
		}
		err = json.NewEncoder(file).Encode(c)
		if err == nil {
			err = file.Sync()
		}
		closeErr := file.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
		// Retain the private request on uncertain transport errors for exact retry.
		if _, err = call(ctx, o, "POST", "/grants/command", c.Issue); err != nil {
			return fmt.Errorf("credential request retained at %s; retry grant activate --credential FILE: %w", path, err)
		}
		return json.NewEncoder(out).Encode(map[string]any{"id": id, "credential_file": path, "status": "grant_recorded"})
	}
	if verb != "client" || !model.Contains([]string{"task", "history", "context", "checkpoint"}, action) {
		return fmt.Errorf("unsupported grant/client action")
	}
	kind := f.String("kind", "checkpoint", "history kind")
	limit := f.Int("limit", 20, "history page size")
	cursor := f.String("cursor", "", "history cursor")
	budget := f.Int("budget", 16000, "context estimate budget")
	file := f.String("file", "", "checkpoint input JSON")
	revision := f.Int64("expected-task-revision", 0, "observed task revision")
	if err := f.Parse(args[2:]); err != nil {
		return err
	}
	if f.NArg() != 0 || *credential == "" {
		return fmt.Errorf("client command requires --credential FILE")
	}
	scoped, err := client.New(*credential)
	if err != nil {
		return err
	}
	if action == "checkpoint" {
		if *file == "" || *revision < 1 || !model.OpaqueID.MatchString(o.requestID) {
			return fmt.Errorf("client checkpoint requires --file, --expected-task-revision and explicit --request-id")
		}
		f, err := os.Open(*file)
		if err != nil {
			return err
		}
		defer f.Close()
		raw, err := io.ReadAll(io.LimitReader(f, continuity.MaxRequest+1))
		if err != nil {
			return err
		}
		if len(raw) > continuity.MaxRequest {
			return fmt.Errorf("checkpoint input too large")
		}
		var cp continuity.CheckpointInput
		if err = model.StrictJSON(raw, &cp); err != nil {
			return err
		}
		r := continuity.Request{Version: 1, ID: o.requestID, Op: "checkpoint.record", Target: target, ExpectedTaskRevision: revision, Checkpoint: &cp}
		if err = r.Validate(); err != nil {
			return err
		}
		return print(scoped.Call(ctx, "POST", "/client/checkpoint", r))
	}
	q := url.Values{"target": {target}}
	if action == "history" {
		q.Set("kind", *kind)
		q.Set("limit", strconv.Itoa(*limit))
		if *cursor != "" {
			q.Set("cursor", *cursor)
		}
	}
	if action == "context" {
		q.Set("budget", strconv.Itoa(*budget))
	}
	return print(scoped.Call(ctx, "GET", "/client/"+action+"?"+q.Encode(), nil))
}

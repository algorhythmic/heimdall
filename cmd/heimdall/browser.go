package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"heimdall/internal/browser"
	"heimdall/internal/nativebridge"
	"io"
	"os"
)

func browserCLI(ctx context.Context, o options, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: browser status|setup|pair|unpair|open|navigate|focus|move|close")
	}
	action := args[0]
	args = args[1:]
	if action == "status" {
		if len(args) != 0 {
			return fmt.Errorf("status takes no arguments")
		}
		b, err := call(ctx, o, "GET", "/browser/state", nil)
		if err == nil {
			_, err = fmt.Fprintln(out, string(b))
		}
		return err
	}
	f := flag.NewFlagSet("browser "+action, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	if action == "setup" {
		id := f.String("extension-id", "", "extension ID")
		dest := f.String("output", "", "new final installation directory")
		if err := f.Parse(args); err != nil {
			return err
		}
		if f.NArg() != 0 || *dest == "" {
			return fmt.Errorf("setup requires --extension-id ID --output DIR")
		}
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		r, err := nativebridge.Prepare(o.dir, *id, *dest, exe)
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(r)
	}
	var c browser.Control
	c.Action = action
	if action == "pair" || action == "unpair" {
		if len(args) != 1 {
			return fmt.Errorf("%s requires profile ID", action)
		}
		c.Profile = args[0]
	} else {
		f.StringVar(&c.Profile, "profile", "", "profile ID")
		f.StringVar(&c.Epoch, "epoch", "", "current epoch")
		f.StringVar(&c.URL, "url", "", "destination URL")
		f.StringVar(&c.ExpectedURL, "expected-url", "", "current URL")
		f.IntVar(&c.TabID, "tab", 0, "tab ID")
		f.IntVar(&c.WindowID, "window", 0, "destination window")
		if err := f.Parse(args); err != nil {
			return err
		}
		if f.NArg() != 0 {
			return fmt.Errorf("unexpected arguments")
		}
	}
	c.ID = o.requestID
	if c.ID == "" {
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			return err
		}
		c.ID = hex.EncodeToString(id[:])
	}
	b, err := call(ctx, o, "POST", "/browser/control", c)
	if err == nil {
		_, err = fmt.Fprintln(out, string(b))
	}
	return err
}

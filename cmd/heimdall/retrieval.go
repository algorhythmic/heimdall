package main

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/model"
	"heimdall/internal/retrieval"
	"io"
	"strings"
)

// braidCLI drives the supervised Braid dataset: publish the permitted
// projection, then query with full why provenance.
func braidCLI(ctx context.Context, o options, args []string, out io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("braid publish | braid query TEXT [--json]")
	}
	switch args[0] {
	case "publish":
		st, err := daemonState(ctx, o)
		if err != nil {
			return err
		}
		c := &retrieval.Client{Dir: o.dir}
		if err := c.Open(ctx); err != nil {
			return err
		}
		defer c.Close()
		res, err := c.Publish(ctx, st)
		if err != nil {
			return err
		}
		if o.json {
			return json.NewEncoder(out).Encode(res)
		}
		fmt.Fprintf(out, "Published Heimdall snapshot to braid dataset (revision advanced).\n")
		return nil
	case "query":
		if len(args) < 2 {
			return fmt.Errorf("braid query TEXT")
		}
		text := strings.Join(args[1:], " ")
		c := &retrieval.Client{Dir: o.dir}
		if err := c.Open(ctx); err != nil {
			return err
		}
		defer c.Close()
		res, err := c.Query(ctx, text, 10)
		if err != nil {
			return err
		}
		if o.json {
			return json.NewEncoder(out).Encode(res)
		}
		items, _ := res["items"].([]any)
		for _, it := range items {
			m, _ := it.(map[string]any)
			fmt.Fprintf(out, "%-40s  %.3f  %s\n", m["id"], m["score"], strings.Join(toStrings(m["why"]), "; "))
		}
		return nil
	default:
		return fmt.Errorf("braid publish | braid query TEXT")
	}
}

func toStrings(v any) []string {
	out := []string{}
	if xs, ok := v.([]any); ok {
		for _, x := range xs {
			out = append(out, fmt.Sprint(x))
		}
	}
	return out
}

func daemonState(ctx context.Context, o options) (model.State, error) {
	raw, err := call(ctx, o, "GET", "/state", nil)
	if err != nil {
		return model.State{}, err
	}
	var st model.State
	if err := model.StrictJSON(raw, &st); err != nil {
		return model.State{}, err
	}
	return st, nil
}

package main

import (
	"context"
	"fmt"
	"io"
)

func uiCLI(ctx context.Context, o options, args []string, out io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("ui requires a root TASK; enter the printed code at the returned URL")
	}
	raw, err := call(ctx, o, "POST", "/ui-bootstrap", map[string]string{"target": args[0]})
	if err == nil {
		_, err = fmt.Fprintln(out, string(raw))
	}
	return err
}

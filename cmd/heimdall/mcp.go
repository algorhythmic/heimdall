package main

import (
	"context"
	"flag"
	"fmt"
	"heimdall/internal/mcpbridge"
	"io"
)

func mcpCLI(ctx context.Context, args []string) error {
	f := flag.NewFlagSet("mcp", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	credential := f.String("credential", "", "scoped credential file")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *credential == "" {
		return fmt.Errorf("mcp --credential FILE required")
	}
	return mcpbridge.Serve(ctx, *credential)
}

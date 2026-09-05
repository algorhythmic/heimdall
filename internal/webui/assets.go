package webui

import "embed"

// Assets are checked in so a Go-only fresh clone can build the daemon.
//
//go:embed assets/*
var Assets embed.FS

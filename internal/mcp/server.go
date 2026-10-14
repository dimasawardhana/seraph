package mcp

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"seraph/internal/board"
	"seraph/internal/db"
	"seraph/internal/repo"
	"seraph/internal/skew"
)

func Serve(ctx context.Context, r repo.Root, version string) error {
	handle, err := db.Open(r)
	if err != nil {
		return err
	}
	defer handle.Close()

	// A clone arrives with the board's history in board.json and no database. Restoring it
	// is not optional here: carrying on with an empty board would let the first tool call
	// render that history away, so a snapshot this build cannot read stops the server.
	if _, err := board.SeedFromSnapshot(handle, r); err != nil {
		return err
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "seraph",
		Title:   "Seraph",
		Version: version,
	}, nil)
	registerTools(server, handle, r)

	// A serve process renders with its start-time binary (TASK-136), so a rebuild
	// reaches a running session only at restart. Stdout is the MCP protocol channel,
	// so the report goes to stderr, where harnesses route server logs. It fires once
	// per detected change, not on every poll — a long-lived server must not spam its
	// host with a condition only a restart fixes.
	if record, err := skew.Capture(); err == nil {
		go func() {
			reported := false
			tick := time.NewTicker(30 * time.Second)
			defer tick.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-tick.C:
					changed, message := record.Changed()
					if changed && !reported {
						fmt.Fprintf(os.Stderr, "seraph: %s\n", message)
						reported = true
					}
				}
			}
		}()
	}
	// A startup that cannot stat its own executable has no identity to watch against;
	// serving anyway is the right call, the watcher is the loss.
	return server.Run(ctx, &mcp.StdioTransport{})
}

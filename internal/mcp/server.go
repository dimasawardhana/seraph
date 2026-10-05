package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"seraph/internal/board"
	"seraph/internal/db"
	"seraph/internal/repo"
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

	return server.Run(ctx, &mcp.StdioTransport{})
}

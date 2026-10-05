package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"seraph/internal/db"
	"seraph/internal/repo"
)

func Serve(ctx context.Context, r repo.Root, version string) error {
	handle, err := db.Open(r)
	if err != nil {
		return err
	}
	defer handle.Close()

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "seraph",
		Title:   "Seraph",
		Version: version,
	}, nil)
	registerTools(server, handle, r)

	return server.Run(ctx, &mcp.StdioTransport{})
}

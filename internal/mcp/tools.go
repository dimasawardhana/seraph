package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"seraph/internal/board"
	"seraph/internal/repo"
)

type noInputs struct{}

type createArgs struct {
	Title       string `json:"title"`
	Goal        string `json:"goal"`
	Acceptance  string `json:"acceptance"`
	Description string `json:"description,omitempty"`
	Priority    string `json:"priority,omitempty"`
	Triage      string `json:"triage,omitempty"`
}

type claimArgs struct {
	TaskID    string `json:"task_id"`
	HarnessID string `json:"harness_id"`
	SessionID string `json:"session_id"`
}

type getTaskArgs struct {
	TaskID string `json:"task_id"`
}

type updateArgs struct {
	TaskID      string  `json:"task_id"`
	SessionID   string  `json:"session_id"`
	Status      *string `json:"status,omitempty"`
	Title       *string `json:"title,omitempty"`
	Goal        *string `json:"goal,omitempty"`
	Description *string `json:"description,omitempty"`
	Acceptance  *string `json:"acceptance,omitempty"`
	Priority    *string `json:"priority,omitempty"`
	Triage      *string `json:"triage,omitempty"`
}

type releaseArgs struct {
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`
}

func registerTools(server *mcp.Server, handle *sql.DB, r repo.Root) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_board_summary",
		Description: "The project's board, grouped by status, in reading order. Returns the same " +
			"text as .seraph/KANBAN.md and rewrites that file in the same call. Read this before " +
			"creating work, so you do not duplicate what is already tracked.",
	}, getBoardSummary(handle, r))

	mcp.AddTool(server, &mcp.Tool{
		Name: "get_task",
		Description: "One task in full, including who holds its claim and until when, as JSON. " +
			"Use this when you have a task id and need detail the board summary omits.",
	}, getTask(handle))

	mcp.AddTool(server, &mcp.Tool{
		Name: "create_task",
		Description: "Add a task with its goal and its acceptance criteria. It starts in backlog. " +
			"Claiming is a separate step, done when work actually starts. Returns the board.",
	}, createTask(handle, r))

	mcp.AddTool(server, &mcp.Tool{
		Name: "claim_task",
		Description: "Take exclusive hold of a task for this session, so another harness cannot " +
			"start the same work. Fails with CLAIM_HELD_BY_ANOTHER_SESSION if someone else holds " +
			"it, and the error names them — report that rather than working around it. An expired " +
			"claim is taken over automatically, which is how a crashed session's work frees itself.",
	}, claimTask(handle, r))

	mcp.AddTool(server, &mcp.Tool{
		Name: "update_task",
		Description: "Change a task's status, text, goal, acceptance criteria, or triage. " +
			"Setting status to done is refused " +
			"unless this session holds the task's claim — that is the only rule the server " +
			"enforces; every other transition is open. Completing a task releases the claim. " +
			"Returns the board.",
	}, updateTask(handle, r))

	mcp.AddTool(server, &mcp.Tool{
		Name: "release_task",
		Description: "Give up a claim without completing the work, so another harness can pick it " +
			"up. Only the session that holds the claim can release it. Returns the board.",
	}, releaseTask(handle, r))
}

func getBoardSummary(handle *sql.DB, r repo.Root) func(context.Context, *mcp.CallToolRequest, noInputs) (*mcp.CallToolResult, any, error) {
	return func(_ context.Context, _ *mcp.CallToolRequest, _ noInputs) (*mcp.CallToolResult, any, error) {
		_, markdown, err := board.Sync(handle, r)
		return respond(markdown, err)
	}
}

func getTask(handle *sql.DB) func(context.Context, *mcp.CallToolRequest, getTaskArgs) (*mcp.CallToolResult, any, error) {
	return func(_ context.Context, _ *mcp.CallToolRequest, args getTaskArgs) (*mcp.CallToolResult, any, error) {
		task, err := board.Get(handle, args.TaskID)
		if err != nil {
			return respond(nil, err)
		}
		detail, err := board.JSONDetail(task)
		if err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(detail)}},
		}, nil, nil
	}
}

func createTask(handle *sql.DB, r repo.Root) func(context.Context, *mcp.CallToolRequest, createArgs) (*mcp.CallToolResult, any, error) {
	return func(_ context.Context, _ *mcp.CallToolRequest, args createArgs) (*mcp.CallToolResult, any, error) {
		_, markdown, err := board.Create(handle, r, board.CreateParams{
			Title:       args.Title,
			Goal:        args.Goal,
			Description: args.Description,
			Acceptance:  args.Acceptance,
			Priority:    args.Priority,
			Triage:      args.Triage,
		})
		return respond(markdown, err)
	}
}

func claimTask(handle *sql.DB, r repo.Root) func(context.Context, *mcp.CallToolRequest, claimArgs) (*mcp.CallToolResult, any, error) {
	return func(_ context.Context, _ *mcp.CallToolRequest, args claimArgs) (*mcp.CallToolResult, any, error) {
		_, markdown, err := board.ClaimTask(handle, r, board.ClaimParams{
			TaskID:  args.TaskID,
			Harness: args.HarnessID,
			Session: args.SessionID,
		})
		// Warn, never refuse. Harnesses differ in what they can pass, and refusing a
		// mismatch would lock out every caller that cannot supply the ambient value —
		// the same deadlock create_task once caused by being classified as a write.
		// What the caller must not do is discover the mismatch only when a gate refuses
		// a write it was entitled to make.
		if err == nil {
			if ambient := os.Getenv("SERAPH_SESSION_ID"); ambient != "" && ambient != args.SessionID {
				markdown = append(markdown, fmt.Sprintf(
					"\n\n> seraph: this claim was taken under session `%s`, but the gate reads `%s` from the"+
						" environment. Use that id in claim_task, update_task and release_task, or your writes"+
						" will be refused as though you held nothing.\n",
					args.SessionID, ambient)...)
			}
		}
		return respond(markdown, err)
	}
}

func updateTask(handle *sql.DB, r repo.Root) func(context.Context, *mcp.CallToolRequest, updateArgs) (*mcp.CallToolResult, any, error) {
	return func(_ context.Context, _ *mcp.CallToolRequest, args updateArgs) (*mcp.CallToolResult, any, error) {
		_, markdown, err := board.UpdateTask(handle, r, board.UpdateParams{
			TaskID:      args.TaskID,
			Session:     args.SessionID,
			Status:      args.Status,
			Title:       args.Title,
			Goal:        args.Goal,
			Description: args.Description,
			Acceptance:  args.Acceptance,
			Priority:    args.Priority,
			Triage:      args.Triage,
		})
		return respond(markdown, err)
	}
}

func releaseTask(handle *sql.DB, r repo.Root) func(context.Context, *mcp.CallToolRequest, releaseArgs) (*mcp.CallToolResult, any, error) {
	return func(_ context.Context, _ *mcp.CallToolRequest, args releaseArgs) (*mcp.CallToolResult, any, error) {
		_, markdown, err := board.ReleaseTask(handle, r, board.ReleaseParams{
			TaskID:  args.TaskID,
			Session: args.SessionID,
		})
		return respond(markdown, err)
	}
}

func respond(markdown []byte, err error) (*mcp.CallToolResult, any, error) {
	if err == nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(markdown)}},
		}, nil, nil
	}

	typed, ok := board.AsError(err)
	if !ok {
		return nil, nil, err
	}
	payload, marshalErr := json.Marshal(typed)
	if marshalErr != nil {
		return nil, nil, marshalErr
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: string(payload)}},
	}, nil, nil
}

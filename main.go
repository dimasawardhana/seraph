package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"seraph/internal/board"
	"seraph/internal/doctor"
	"seraph/internal/gate"
	"seraph/internal/installer"
	"seraph/internal/mcp"
	"seraph/internal/repo"
	"seraph/internal/tui"
	"seraph/internal/version"
	"seraph/internal/webui"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	board.ConfigureFromEnv()

	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "seraph:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	command := "serve"
	if len(args) > 0 {
		command = args[0]
	}

	switch command {
	case "serve":
		root, err := repo.Resolve(".")
		if err != nil {
			return err
		}
		return mcp.Serve(ctx, root, version.String)
	case "board":
		root, err := repo.Resolve(".")
		if err != nil {
			return err
		}
		return tui.Run(root)
	case "ui":
		return runUI(ctx, args[1:])
	case "hook":
		return runHook(args[1:])
	case "gate":
		os.Exit(gate.Run(args[1:], os.Stdin, os.Stdout, os.Stderr))
		return nil
	case "install":
		return runInstall(args[1:])
	case "doctor":
		return doctor.Run(ctx, os.Stdout)
	case "version":
		fmt.Fprintln(os.Stdout, version.String)
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", command)
	}
}

func runUI(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("ui", flag.ContinueOnError)
	port := flags.Int("port", 7777, "port to listen on, loopback only")
	if err := flags.Parse(args); err != nil {
		return err
	}

	root, err := repo.Resolve(".")
	if err != nil {
		return err
	}
	url, err := webui.Listen(root, *port)
	if err != nil {
		return err
	}
	fmt.Printf("seraph board at %s\n", url)
	fmt.Println("bound to 127.0.0.1 — the token in that URL is required for every request")
	fmt.Println("Ctrl-C to stop")

	<-ctx.Done()
	return nil
}

func runInstall(args []string) error {
	dryRun := false
	for _, arg := range args {
		switch arg {
		case "--dry-run", "-n":
			dryRun = true
		default:
			usage()
			return fmt.Errorf("unknown option %q", arg)
		}
	}

	root, err := repo.Resolve(".")
	if err != nil {
		return err
	}
	changes, err := installer.Install(root, dryRun)
	if err != nil {
		return err
	}
	installer.Report(os.Stdout, changes, dryRun)

	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "claim gate — refuses a write from a session holding no live claim")
	explain := false
	for _, a := range args {
		if a == "--explain" {
			explain = true
		}
	}
	for _, h := range installer.Hooks() {
		suffix := ""
		if explain {
			suffix = "  · " + h.Doc
		}
		fmt.Fprintf(os.Stdout, "  %-15s %-22s %s\n", h.Status, h.Name, h.Detail+suffix)
	}
	fmt.Fprintln(os.Stdout, "\n  run `seraph hook --explain` for the shape each harness expects")
	return nil
}

// runHook reports which harnesses can enforce the gate and which cannot, without writing.
func runHook(args []string) error {
	explain, verify := false, false
	for _, a := range args {
		switch a {
		case "--explain":
			explain = true
		case "--verify":
			verify = true
		default:
			usage()
			return fmt.Errorf("unknown option %q", a)
		}
	}
	if verify {
		return verifyRecorded()
	}
	root, err := repo.Resolve(".")
	if err != nil {
		return err
	}
	reports, err := installer.InspectHooks(root.Path)
	if err != nil {
		return err
	}
	for _, h := range reports {
		fmt.Fprintf(os.Stdout, "  %-22s %-22s %s\n", h.Status, h.Name, h.Path)
		// The detail is why a row reads the way it does — the harness's own limitation,
		// or a gate file left behind by an install that now does nothing. Printing the
		// status without it leaves the reader to infer the reason, which is how a row
		// that says "cannot enforce" ends up read as a verdict with nothing behind it.
		if h.Detail != "" && !explain {
			fmt.Fprintf(os.Stdout, "  %-22s %s\n", "", h.Detail)
		}
		if explain {
			fmt.Fprintf(os.Stdout, "  %-22s %s\n", "", h.Doc)
		}
	}
	if !explain {
		fmt.Fprintln(os.Stdout, "\n  --explain shows the hook shape each harness expects")
	}
	return nil
}

// verifyRecorded reports whether each verified harness still runs the build its entry was
// witnessed on. It never claims enforcement and never edits the registry: watching a new
// refusal is somebody's job, and a command that could promote itself to evidence would be
// the exact defect this project exists to end.
func verifyRecorded() error {
	checks := installer.VerifyRecorded()
	if len(checks) == 0 {
		fmt.Fprintln(os.Stdout, "\n  No harness has a verified entry to check.")
		return nil
	}
	stale := 0
	for _, c := range checks {
		switch {
		case c.Err != "":
			stale++
			fmt.Fprintf(os.Stdout, "  %-22s %-22s could not run its version command: %s\n", "unchecked", c.Harness, c.Err)
		case c.Holds:
			fmt.Fprintf(os.Stdout, "  %-22s %-22s recorded %s, installed reports %q\n", "holds", c.Harness, c.Recorded, c.Installed)
		default:
			stale++
			fmt.Fprintf(os.Stdout, "  %-22s %-22s recorded %s, installed reports %q — watch a refusal again before trusting this\n",
				"STALE", c.Harness, c.Recorded, c.Installed)
		}
	}
	if stale > 0 {
		fmt.Fprintf(os.Stdout, "\n  %d entr%s can no longer be taken from the build it was witnessed on.\n",
			stale, map[bool]string{true: "y", false: "ies"}[stale == 1])
	}
	return nil
}

func usage() {
	fmt.Fprint(os.Stderr, `seraph — a local task board for AI coding harnesses

usage:
  seraph [serve]             serve the board over MCP on stdio (default)
  seraph board               view and work the board in the terminal
  seraph ui [--port N]       view and work the board in a browser (default 7777)
  seraph install [--dry-run] register with this repository's harnesses and place the Rules
  seraph hook [--explain] report which harnesses can enforce the claim gate
  seraph gate            the pre-tool hook a harness runs; not usually called by hand
  seraph doctor              report repository, database, and snapshot health
  seraph version             print the version

  SERAPH_CLAIM_TTL           how long a claim survives, e.g. 45m (default 30m)
`)
}

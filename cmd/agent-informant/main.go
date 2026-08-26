package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/MDragonryu/Agent-Informant/internal/usage"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printHelp()
		return 0
	}

	switch args[0] {
	case "usage":
		return runUsage(args[1:])
	case "version":
		fmt.Println("agent-informant dev")
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown domain %q\n\n", args[0])
		printHelp()
		return 2
	}
}

func runUsage(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printUsageHelp()
		return 0
	}

	switch args[0] {
	case "status":
		return runUsageStatus(args[1:])
	case "advise":
		return runUsageAdvise(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown usage command %q\n\n", args[0])
		printUsageHelp()
		return 2
	}
}

func commonFlags(name string, args []string) (*flag.FlagSet, *string, *string, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	provider := fs.String("provider", "", "provider to query, e.g. codex or claude")
	format := fs.String("format", "text", "output format: text or json")
	if err := fs.Parse(args); err != nil {
		return nil, nil, nil, err
	}
	if *format != "text" && *format != "json" {
		return nil, nil, nil, fmt.Errorf("invalid --format %q: expected text or json", *format)
	}
	return fs, provider, format, nil
}

func collect(provider string) (usage.Snapshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	collector := usage.NewCodexBarCollector()
	return collector.Collect(ctx, provider)
}

func runUsageStatus(args []string) int {
	_, provider, format, err := commonFlags("usage status", args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	snapshot, err := collect(*provider)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if *format == "json" {
		return printJSON(snapshot)
	}

	for _, w := range snapshot.Windows {
		reset := "unknown"
		if w.ResetAt != nil {
			reset = w.ResetAt.Format(time.RFC3339)
		}
		fmt.Printf("%s %s: %.1f%% remaining (%.1f%% used), reset %s [%s]\n", w.Provider, w.Name, w.PercentRemaining, w.PercentUsed, reset, w.Source)
	}
	return 0
}

func runUsageAdvise(args []string) int {
	fs := flag.NewFlagSet("usage advise", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	provider := fs.String("provider", "", "provider to query, e.g. codex or claude")
	format := fs.String("format", "text", "output format: text or json")
	draining := fs.Float64("draining", 25, "remaining percentage at or below which draining begins")
	critical := fs.Float64("critical", 10, "remaining percentage at or below which critical begins")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintf(os.Stderr, "invalid --format %q: expected text or json\n", *format)
		return 2
	}

	snapshot, err := collect(*provider)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	advice, err := (usage.Policy{DrainingRemaining: *draining, CriticalRemaining: *critical}).Evaluate(snapshot)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if *format == "json" {
		if code := printJSON(advice); code != 0 {
			return code
		}
	} else {
		fmt.Printf("state: %s\naction: %s\nmessage: %s\n", advice.State, advice.Action, advice.Message)
		if advice.WorstWindow != nil {
			w := advice.WorstWindow
			fmt.Printf("limiting-window: %s/%s (%.1f%% remaining)\n", w.Provider, w.Name, w.PercentRemaining)
		}
	}
	return usage.ExitCode(advice.State)
}

func printJSON(v any) int {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func printHelp() {
	fmt.Println(strings.TrimSpace(`Agent Informant

Usage:
  agent-informant <domain> <command> [flags]

Domains:
  usage      Query and interpret provider usage information

Other commands:
  version    Print version information
  help       Show this help`))
}

func printUsageHelp() {
	fmt.Println(strings.TrimSpace(`Usage domain

Commands:
  agent-informant usage status [--provider NAME] [--format text|json]
  agent-informant usage advise [--provider NAME] [--format text|json] [--draining 25] [--critical 10]

Exit codes for advise:
  0   green
  10  draining
  20  critical
  1   usage unavailable or invalid
  2   CLI usage error`))
}

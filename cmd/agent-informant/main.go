package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	appconfig "github.com/MDragonryu/Agent-Informant/internal/config"
	"github.com/MDragonryu/Agent-Informant/internal/delivery"
	"github.com/MDragonryu/Agent-Informant/internal/usage"
)

func main() { os.Exit(run(os.Args[1:])) }

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func run(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printHelp()
		return 0
	}
	switch args[0] {
	case "usage":
		return runUsage(args[1:])
	case "config":
		return runConfig(args[1:])
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
	case "watch":
		return runUsageWatch(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown usage command %q\n\n", args[0])
		printUsageHelp()
		return 2
	}
}

func collect(provider string) (usage.Snapshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	return usage.NewCodexBarCollector().Collect(ctx, provider)
}

func runUsageStatus(args []string) int {
	fs := flag.NewFlagSet("usage status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	provider := fs.String("provider", "", "provider to query, e.g. codex or claude")
	format := fs.String("format", "text", "output format: text or json")
	if err := fs.Parse(args); err != nil { return 2 }
	if *format != "text" && *format != "json" {
		fmt.Fprintf(os.Stderr, "invalid --format %q: expected text or json\n", *format)
		return 2
	}
	snapshot, err := collect(*provider)
	if err != nil { fmt.Fprintln(os.Stderr, err); return 1 }
	if *format == "json" { return printJSON(snapshot) }
	for _, w := range snapshot.Windows {
		reset := "unknown"
		if w.ResetAt != nil { reset = w.ResetAt.Format(time.RFC3339) }
		fmt.Printf("%s %s: %.1f%% remaining (%.1f%% used), reset %s [%s]\n", w.Provider, w.Name, w.PercentRemaining, w.PercentUsed, reset, w.Source)
	}
	return 0
}

func loadPolicy(configPath string, draining, critical float64, green, drainingMsg, criticalMsg string) (usage.Policy, appconfig.Config, error) {
	cfg, _, err := appconfig.Load(configPath)
	if err != nil { return usage.Policy{}, appconfig.Config{}, err }
	if draining >= 0 { cfg.Usage.DrainingRemaining = draining }
	if critical >= 0 { cfg.Usage.CriticalRemaining = critical }
	if green != "" { cfg.Usage.Messages.Green = green }
	if drainingMsg != "" { cfg.Usage.Messages.Draining = drainingMsg }
	if criticalMsg != "" { cfg.Usage.Messages.Critical = criticalMsg }
	if err := appconfig.Validate(cfg); err != nil { return usage.Policy{}, appconfig.Config{}, err }
	return usage.Policy{
		DrainingRemaining: cfg.Usage.DrainingRemaining,
		CriticalRemaining: cfg.Usage.CriticalRemaining,
		Messages: usage.Messages{Green: cfg.Usage.Messages.Green, Draining: cfg.Usage.Messages.Draining, Critical: cfg.Usage.Messages.Critical},
	}, cfg, nil
}

func runUsageAdvise(args []string) int {
	fs := flag.NewFlagSet("usage advise", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	provider := fs.String("provider", "", "provider to query")
	format := fs.String("format", "text", "output format: text or json")
	configPath := fs.String("config", "", "config file path")
	draining := fs.Float64("draining", -1, "override draining remaining percentage")
	critical := fs.Float64("critical", -1, "override critical remaining percentage")
	greenMsg := fs.String("message-green", "", "override green-state message")
	drainingMsg := fs.String("message-draining", "", "override draining-state message")
	criticalMsg := fs.String("message-critical", "", "override critical-state message")
	if err := fs.Parse(args); err != nil { return 2 }
	if *format != "text" && *format != "json" { fmt.Fprintf(os.Stderr, "invalid --format %q\n", *format); return 2 }
	policy, _, err := loadPolicy(*configPath, *draining, *critical, *greenMsg, *drainingMsg, *criticalMsg)
	if err != nil { fmt.Fprintln(os.Stderr, err); return 1 }
	snapshot, err := collect(*provider)
	if err != nil { fmt.Fprintln(os.Stderr, err); return 1 }
	advice, err := policy.Evaluate(snapshot)
	if err != nil { fmt.Fprintln(os.Stderr, err); return 1 }
	if *format == "json" {
		if code := printJSON(advice); code != 0 { return code }
	} else {
		printAdvice(advice)
	}
	return usage.ExitCode(advice.State)
}

func runUsageWatch(args []string) int {
	fs := flag.NewFlagSet("usage watch", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	provider := fs.String("provider", "", "provider to query")
	format := fs.String("format", "text", "output format: text or jsonl")
	configPath := fs.String("config", "", "config file path")
	interval := fs.Int("interval", -1, "poll interval in seconds")
	draining := fs.Float64("draining", -1, "override draining remaining percentage")
	critical := fs.Float64("critical", -1, "override critical remaining percentage")
	greenMsg := fs.String("message-green", "", "override green-state message")
	drainingMsg := fs.String("message-draining", "", "override draining-state message")
	criticalMsg := fs.String("message-critical", "", "override critical-state message")
	execPath := fs.String("exec", "", "deliver each emitted watch event to this executable")
	var execArgs stringList
	fs.Var(&execArgs, "exec-arg", "argument passed to --exec; repeat for multiple arguments")
	execTimeout := fs.Int("exec-timeout", 10, "delivery executable timeout in seconds")
	noOutput := fs.Bool("no-output", false, "suppress normal watch stdout; useful with --exec")
	if err := fs.Parse(args); err != nil { return 2 }
	if *format != "text" && *format != "jsonl" { fmt.Fprintf(os.Stderr, "invalid --format %q: expected text or jsonl\n", *format); return 2 }
	if *execTimeout < 1 { fmt.Fprintln(os.Stderr, "--exec-timeout must be at least 1 second"); return 2 }
	if *noOutput && *execPath == "" { fmt.Fprintln(os.Stderr, "--no-output requires --exec"); return 2 }

	policy, cfg, err := loadPolicy(*configPath, *draining, *critical, *greenMsg, *drainingMsg, *criticalMsg)
	if err != nil { fmt.Fprintln(os.Stderr, err); return 1 }
	poll := cfg.Usage.WatchIntervalSec
	if *interval >= 0 { poll = *interval }
	if poll < 1 { fmt.Fprintln(os.Stderr, "--interval must be at least 1 second"); return 2 }

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	watcher := usage.Watcher{Collector: usage.NewCodexBarCollector(), Policy: policy, Provider: *provider, Interval: time.Duration(poll) * time.Second}
	enc := json.NewEncoder(os.Stdout)
	var hook *delivery.Exec
	if *execPath != "" {
		hook = &delivery.Exec{Path: *execPath, Args: execArgs, Timeout: time.Duration(*execTimeout) * time.Second}
	}

	err = watcher.Run(ctx, func(event usage.WatchEvent) error {
		if hook != nil {
			payload, marshalErr := json.Marshal(event)
			if marshalErr != nil {
				fmt.Fprintf(os.Stderr, "encode hook event: %v\n", marshalErr)
			} else {
				payload = append(payload, '\n')
				if hookErr := hook.Send(ctx, payload, watchEventEnv(event)); hookErr != nil {
					fmt.Fprintf(os.Stderr, "deliver watch event: %v\n", hookErr)
				}
			}
		}

		if *noOutput { return nil }
		if *format == "jsonl" { return enc.Encode(event) }
		printWatchEvent(event)
		return nil
	})
	if err != nil { fmt.Fprintln(os.Stderr, err); return 1 }
	return 0
}

func watchEventEnv(event usage.WatchEvent) map[string]string {
	env := map[string]string{
		"AGENT_INFORMANT_EVENT":       string(event.Type),
		"AGENT_INFORMANT_OBSERVED_AT": event.ObservedAt.Format(time.RFC3339Nano),
	}
	if event.PreviousState != nil {
		env["AGENT_INFORMANT_PREVIOUS_STATE"] = string(*event.PreviousState)
	}
	if event.Error != "" {
		env["AGENT_INFORMANT_ERROR"] = event.Error
	}
	if event.Advice == nil {
		return env
	}

	a := event.Advice
	env["AGENT_INFORMANT_STATE"] = string(a.State)
	env["AGENT_INFORMANT_ACTION"] = a.Action
	env["AGENT_INFORMANT_MESSAGE"] = a.Message
	if a.WorstWindow != nil {
		w := a.WorstWindow
		env["AGENT_INFORMANT_PROVIDER"] = w.Provider
		env["AGENT_INFORMANT_WINDOW"] = w.Name
		env["AGENT_INFORMANT_PERCENT_REMAINING"] = strconv.FormatFloat(w.PercentRemaining, 'f', -1, 64)
		env["AGENT_INFORMANT_PERCENT_USED"] = strconv.FormatFloat(w.PercentUsed, 'f', -1, 64)
		if w.ResetAt != nil {
			env["AGENT_INFORMANT_RESET_AT"] = w.ResetAt.Format(time.RFC3339Nano)
		}
	}
	return env
}

func printWatchEvent(event usage.WatchEvent) {
	if event.Type == usage.WatchError {
		fmt.Printf("error %s\n", event.Error)
		return
	}
	if event.Advice == nil { return }
	a := event.Advice
	remaining := "unknown"
	window := "unknown"
	if a.WorstWindow != nil {
		remaining = fmt.Sprintf("%.1f%%", a.WorstWindow.PercentRemaining)
		window = a.WorstWindow.Provider + "/" + a.WorstWindow.Name
	}
	prefix := string(event.Type)
	if event.PreviousState != nil { prefix += ":" + string(*event.PreviousState) + "->" + string(a.State) }
	fmt.Printf("%s %s %s %s %s | %s\n", prefix, a.State, remaining, window, a.Action, a.Message)
}

func printAdvice(advice usage.Advice) {
	fmt.Printf("state: %s\naction: %s\nmessage: %s\n", advice.State, advice.Action, advice.Message)
	if advice.WorstWindow != nil {
		w := advice.WorstWindow
		fmt.Printf("limiting-window: %s/%s (%.1f%% remaining)\n", w.Provider, w.Name, w.PercentRemaining)
	}
}

func runConfig(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printConfigHelp(); return 0
	}
	switch args[0] {
	case "path":
		path, err := appconfig.DefaultPath(); if err != nil { fmt.Fprintln(os.Stderr, err); return 1 }; fmt.Println(path); return 0
	case "show":
		fs := flag.NewFlagSet("config show", flag.ContinueOnError); path := fs.String("config", "", "config file path"); if err := fs.Parse(args[1:]); err != nil { return 2 }
		cfg, resolved, err := appconfig.Load(*path); if err != nil { fmt.Fprintln(os.Stderr, err); return 1 }
		fmt.Fprintf(os.Stderr, "config: %s\n", resolved); return printJSON(cfg)
	case "init":
		fs := flag.NewFlagSet("config init", flag.ContinueOnError); path := fs.String("config", "", "config file path"); force := fs.Bool("force", false, "replace existing config"); if err := fs.Parse(args[1:]); err != nil { return 2 }
		resolved, err := appconfig.WriteDefault(*path, *force); if err != nil { fmt.Fprintln(os.Stderr, err); return 1 }; fmt.Println(resolved); return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown config command %q\n", args[0]); return 2
	}
}

func printJSON(v any) int {
	enc := json.NewEncoder(os.Stdout); enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil { fmt.Fprintln(os.Stderr, err); return 1 }
	return 0
}

func printHelp() {
	fmt.Println(strings.TrimSpace(`Agent Informant

Usage:
  agent-informant <domain> <command> [flags]

Domains:
  usage      Query and interpret provider usage information
  config     Inspect or initialize Agent Informant configuration

Other commands:
  version    Print version information
  help       Show this help`))
}

func printUsageHelp() {
	fmt.Println(strings.TrimSpace(`Usage domain

Commands:
  agent-informant usage status [--provider NAME] [--format text|json]
  agent-informant usage advise [--provider NAME] [--format text|json]
  agent-informant usage watch  [--provider NAME] [--format text|jsonl] [--interval SECONDS]

Advice and watch use thresholds/messages from the config file. Flags such as --draining,
--critical, --message-green, --message-draining, and --message-critical override them.

Watch emits one initial event and then only state changes or collection errors.
Use --exec PATH to deliver each emitted event to an executable as JSON on stdin.
Repeat --exec-arg VALUE for arguments. --no-output makes watch hook-only.

Exit codes for advise:
  0   green
  10  draining
  20  critical
  1   usage unavailable or invalid
  2   CLI usage error`))
}

func printConfigHelp() {
	fmt.Println(strings.TrimSpace(`Configuration

Commands:
  agent-informant config path
  agent-informant config show [--config PATH]
  agent-informant config init [--config PATH] [--force]`))
}

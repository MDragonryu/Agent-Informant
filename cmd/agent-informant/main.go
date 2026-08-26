package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	appconfig "github.com/MDragonryu/Agent-Informant/internal/config"
	"github.com/MDragonryu/Agent-Informant/internal/delivery"
	setuphooks "github.com/MDragonryu/Agent-Informant/internal/setup"
	"github.com/MDragonryu/Agent-Informant/internal/usage"
)

func main() { os.Exit(run(os.Args[1:])) }

type stringList []string
func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

func run(args []string) int {
	if len(args) == 0 || isHelp(args[0]) { printHelp(); return 0 }
	switch args[0] {
	case "usage": return runUsage(args[1:])
	case "config": return runConfig(args[1:])
	case "setup": return runSetup(args[1:])
	case "version": fmt.Println("agent-informant dev"); return 0
	default: fmt.Fprintf(os.Stderr, "unknown domain %q\n\n", args[0]); printHelp(); return 2
	}
}

func isHelp(s string) bool { return s == "help" || s == "--help" || s == "-h" }

func runUsage(args []string) int {
	if len(args) == 0 || isHelp(args[0]) { printUsageHelp(); return 0 }
	switch args[0] {
	case "status": return runUsageStatus(args[1:])
	case "advise": return runUsageAdvise(args[1:])
	case "watch": return runUsageWatch(args[1:])
	case "hook": return runUsageHook(args[1:])
	default: fmt.Fprintf(os.Stderr, "unknown usage command %q\n\n", args[0]); printUsageHelp(); return 2
	}
}

func collect(provider string) (usage.Snapshot, error) { return collectTimeout(provider, 90*time.Second) }
func collectTimeout(provider string, timeout time.Duration) (usage.Snapshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout); defer cancel()
	return usage.NewCodexBarCollector().Collect(ctx, provider)
}

func runUsageStatus(args []string) int {
	fs := flag.NewFlagSet("usage status", flag.ContinueOnError); fs.SetOutput(os.Stderr)
	provider := fs.String("provider", "", "provider to query")
	format := fs.String("format", "text", "output format: text or json")
	if fs.Parse(args) != nil { return 2 }
	if *format != "text" && *format != "json" { fmt.Fprintln(os.Stderr, "--format must be text or json"); return 2 }
	s, err := collect(*provider); if err != nil { fmt.Fprintln(os.Stderr, err); return 1 }
	if *format == "json" { return printJSON(s) }
	for _, w := range s.Windows {
		reset := "unknown"; if w.ResetAt != nil { reset = w.ResetAt.Format(time.RFC3339) }
		fmt.Printf("%s %s: %.1f%% remaining (%.1f%% used), reset %s [%s]\n", w.Provider, w.Name, w.PercentRemaining, w.PercentUsed, reset, w.Source)
	}
	return 0
}

func loadPolicy(configPath string, draining, critical float64, green, drainingMsg, criticalMsg string) (usage.Policy, appconfig.Config, error) {
	cfg, _, err := appconfig.Load(configPath); if err != nil { return usage.Policy{}, appconfig.Config{}, err }
	if draining >= 0 { cfg.Usage.DrainingRemaining = draining }
	if critical >= 0 { cfg.Usage.CriticalRemaining = critical }
	if green != "" { cfg.Usage.Messages.Green = green }
	if drainingMsg != "" { cfg.Usage.Messages.Draining = drainingMsg }
	if criticalMsg != "" { cfg.Usage.Messages.Critical = criticalMsg }
	if err := appconfig.Validate(cfg); err != nil { return usage.Policy{}, appconfig.Config{}, err }
	return usage.Policy{DrainingRemaining: cfg.Usage.DrainingRemaining, CriticalRemaining: cfg.Usage.CriticalRemaining,
		Messages: usage.Messages{Green: cfg.Usage.Messages.Green, Draining: cfg.Usage.Messages.Draining, Critical: cfg.Usage.Messages.Critical}}, cfg, nil
}

func runUsageAdvise(args []string) int {
	fs := flag.NewFlagSet("usage advise", flag.ContinueOnError); fs.SetOutput(os.Stderr)
	provider := fs.String("provider", "", "provider to query"); format := fs.String("format", "text", "text or json")
	configPath := fs.String("config", "", "config path"); draining := fs.Float64("draining", -1, "override draining threshold")
	critical := fs.Float64("critical", -1, "override critical threshold"); greenMsg := fs.String("message-green", "", "override green message")
	drainingMsg := fs.String("message-draining", "", "override draining message"); criticalMsg := fs.String("message-critical", "", "override critical message")
	if fs.Parse(args) != nil { return 2 }; if *format != "text" && *format != "json" { return 2 }
	policy, _, err := loadPolicy(*configPath, *draining, *critical, *greenMsg, *drainingMsg, *criticalMsg); if err != nil { fmt.Fprintln(os.Stderr, err); return 1 }
	s, err := collect(*provider); if err != nil { fmt.Fprintln(os.Stderr, err); return 1 }
	a, err := policy.Evaluate(s); if err != nil { fmt.Fprintln(os.Stderr, err); return 1 }
	if *format == "json" { if c := printJSON(a); c != 0 { return c } } else { printAdvice(a) }
	return usage.ExitCode(a.State)
}

func runUsageHook(args []string) int {
	fs := flag.NewFlagSet("usage hook", flag.ContinueOnError); fs.SetOutput(io.Discard)
	runtimeName := fs.String("runtime", "", "claude or codex"); configPath := fs.String("config", "", "config path")
	if fs.Parse(args) != nil { return 0 }
	provider := ""
	switch *runtimeName { case "claude": provider = "claude"; case "codex": provider = "codex"; default: return 0 }
	input, err := io.ReadAll(os.Stdin); if err != nil { hookDebug(err); return 0 }
	event, err := usage.ParseRuntimeHookEvent(input); if err != nil { hookDebug(err); return 0 }
	policy, _, err := loadPolicy(*configPath, -1, -1, "", "", ""); if err != nil { hookDebug(err); return 0 }
	s, err := collectTimeout(provider, 12*time.Second); if err != nil { hookDebug(err); return 0 }
	a, err := policy.Evaluate(s); if err != nil { hookDebug(err); return 0 }
	out, err := usage.RuntimeHookOutput(event, a); if err != nil { hookDebug(err); return 0 }
	if len(out) > 0 { _, _ = os.Stdout.Write(append(out, '\n')) }
	return 0
}

func hookDebug(err error) { if os.Getenv("AGENT_INFORMANT_HOOK_DEBUG") != "" { fmt.Fprintf(os.Stderr, "agent-informant hook: %v\n", err) } }

func runUsageWatch(args []string) int {
	fs := flag.NewFlagSet("usage watch", flag.ContinueOnError); fs.SetOutput(os.Stderr)
	provider := fs.String("provider", "", "provider to query"); format := fs.String("format", "text", "text or jsonl")
	configPath := fs.String("config", "", "config path"); interval := fs.Int("interval", -1, "poll seconds")
	draining := fs.Float64("draining", -1, "override draining threshold"); critical := fs.Float64("critical", -1, "override critical threshold")
	greenMsg := fs.String("message-green", "", "override green message"); drainingMsg := fs.String("message-draining", "", "override draining message")
	criticalMsg := fs.String("message-critical", "", "override critical message"); execPath := fs.String("exec", "", "event executable")
	var execArgs stringList; fs.Var(&execArgs, "exec-arg", "repeatable executable arg")
	execTimeout := fs.Int("exec-timeout", 10, "event executable timeout seconds"); noOutput := fs.Bool("no-output", false, "suppress watcher output")
	if fs.Parse(args) != nil { return 2 }; if *format != "text" && *format != "jsonl" { return 2 }
	if *execTimeout < 1 || (*noOutput && *execPath == "") { return 2 }
	policy, cfg, err := loadPolicy(*configPath, *draining, *critical, *greenMsg, *drainingMsg, *criticalMsg); if err != nil { fmt.Fprintln(os.Stderr, err); return 1 }
	poll := cfg.Usage.WatchIntervalSec; if *interval >= 0 { poll = *interval }; if poll < 1 { return 2 }
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM); defer stop()
	watcher := usage.Watcher{Collector: usage.NewCodexBarCollector(), Policy: policy, Provider: *provider, Interval: time.Duration(poll)*time.Second}
	enc := json.NewEncoder(os.Stdout); var hook *delivery.Exec
	if *execPath != "" { hook = &delivery.Exec{Path:*execPath, Args:execArgs, Timeout:time.Duration(*execTimeout)*time.Second} }
	err = watcher.Run(ctx, func(event usage.WatchEvent) error {
		if hook != nil { if b,e:=json.Marshal(event); e==nil { if e=hook.Send(ctx, append(b,'\n'), watchEventEnv(event)); e!=nil { fmt.Fprintf(os.Stderr,"deliver watch event: %v\n",e) } } }
		if *noOutput { return nil }; if *format == "jsonl" { return enc.Encode(event) }; printWatchEvent(event); return nil
	})
	if err != nil { fmt.Fprintln(os.Stderr, err); return 1 }; return 0
}

func runSetup(args []string) int {
	if len(args)==0 || isHelp(args[0]) { printSetupHelp(); return 0 }
	target := args[0]; if target!="claude" && target!="codex" && target!="all" { fmt.Fprintln(os.Stderr,"setup target must be claude, codex, or all"); return 2 }
	fs:=flag.NewFlagSet("setup "+target, flag.ContinueOnError); fs.SetOutput(os.Stderr)
	executable:=fs.String("executable","","agent-informant executable path"); claudePath:=fs.String("claude-path","","override Claude settings.json path"); codexPath:=fs.String("codex-path","","override Codex hooks.json path")
	if fs.Parse(args[1:])!=nil { return 2 }
	install:=func(rt setuphooks.Runtime,path string) bool { p,err:=setuphooks.Install(rt,path,*executable); if err!=nil { fmt.Fprintln(os.Stderr,err); return false }; fmt.Printf("%s: %s\n",rt,p); return true }
	ok:=true; if target=="claude"||target=="all" { ok=install(setuphooks.Claude,*claudePath)&&ok }; if target=="codex"||target=="all" { ok=install(setuphooks.Codex,*codexPath)&&ok }
	if !ok { return 1 }; return 0
}

func watchEventEnv(event usage.WatchEvent) map[string]string {
	env:=map[string]string{"AGENT_INFORMANT_EVENT":string(event.Type),"AGENT_INFORMANT_OBSERVED_AT":event.ObservedAt.Format(time.RFC3339Nano)}
	if event.PreviousState!=nil { env["AGENT_INFORMANT_PREVIOUS_STATE"]=string(*event.PreviousState) }; if event.Error!="" { env["AGENT_INFORMANT_ERROR"]=event.Error }
	if event.Advice==nil{return env}; a:=event.Advice; env["AGENT_INFORMANT_STATE"]=string(a.State); env["AGENT_INFORMANT_ACTION"]=a.Action; env["AGENT_INFORMANT_MESSAGE"]=a.Message
	if a.WorstWindow!=nil { w:=a.WorstWindow; env["AGENT_INFORMANT_PROVIDER"]=w.Provider; env["AGENT_INFORMANT_WINDOW"]=w.Name; env["AGENT_INFORMANT_PERCENT_REMAINING"]=strconv.FormatFloat(w.PercentRemaining,'f',-1,64); env["AGENT_INFORMANT_PERCENT_USED"]=strconv.FormatFloat(w.PercentUsed,'f',-1,64); if w.ResetAt!=nil { env["AGENT_INFORMANT_RESET_AT"]=w.ResetAt.Format(time.RFC3339Nano) } }
	return env
}

func printWatchEvent(event usage.WatchEvent) { if event.Type==usage.WatchError { fmt.Printf("error %s\n",event.Error); return }; if event.Advice==nil{return}; a:=event.Advice; rem,win:="unknown","unknown"; if a.WorstWindow!=nil { rem=fmt.Sprintf("%.1f%%",a.WorstWindow.PercentRemaining); win=a.WorstWindow.Provider+"/"+a.WorstWindow.Name }; prefix:=string(event.Type); if event.PreviousState!=nil { prefix+=":"+string(*event.PreviousState)+"->"+string(a.State) }; fmt.Printf("%s %s %s %s %s | %s\n",prefix,a.State,rem,win,a.Action,a.Message) }
func printAdvice(a usage.Advice) { fmt.Printf("state: %s\naction: %s\nmessage: %s\n",a.State,a.Action,a.Message); if a.WorstWindow!=nil { fmt.Printf("limiting-window: %s/%s (%.1f%% remaining)\n",a.WorstWindow.Provider,a.WorstWindow.Name,a.WorstWindow.PercentRemaining) } }

func runConfig(args []string) int {
	if len(args)==0||isHelp(args[0]) { printConfigHelp(); return 0 }
	switch args[0] {
	case "path": p,e:=appconfig.DefaultPath(); if e!=nil{fmt.Fprintln(os.Stderr,e);return 1}; fmt.Println(p); return 0
	case "show": fs:=flag.NewFlagSet("config show",flag.ContinueOnError); p:=fs.String("config","","config path"); if fs.Parse(args[1:])!=nil{return 2}; c,r,e:=appconfig.Load(*p); if e!=nil{fmt.Fprintln(os.Stderr,e);return 1}; fmt.Fprintf(os.Stderr,"config: %s\n",r); return printJSON(c)
	case "init": fs:=flag.NewFlagSet("config init",flag.ContinueOnError); p:=fs.String("config","","config path"); force:=fs.Bool("force",false,"replace existing"); if fs.Parse(args[1:])!=nil{return 2}; r,e:=appconfig.WriteDefault(*p,*force); if e!=nil{fmt.Fprintln(os.Stderr,e);return 1}; fmt.Println(r); return 0
	default: return 2
	}
}

func printJSON(v any) int { e:=json.NewEncoder(os.Stdout); e.SetIndent("","  "); if err:=e.Encode(v);err!=nil{fmt.Fprintln(os.Stderr,err);return 1}; return 0 }
func printHelp(){fmt.Println(strings.TrimSpace(`Agent Informant

Usage:
  agent-informant <domain> <command> [flags]

Domains:
  usage      Query and interpret provider usage information
  config     Inspect or initialize configuration
  setup      Install structural hooks into agent runtimes

Other commands:
  version
  help`))}
func printUsageHelp(){fmt.Println(strings.TrimSpace(`Usage domain

Commands:
  agent-informant usage status [--provider NAME] [--format text|json]
  agent-informant usage advise [--provider NAME] [--format text|json]
  agent-informant usage watch  [--provider NAME] [--format text|jsonl]
  agent-informant usage hook   --runtime claude|codex

"hook" is intended for lifecycle-hook runtimes, not direct interactive use.`))}
func printConfigHelp(){fmt.Println(strings.TrimSpace(`Configuration

Commands:
  agent-informant config path
  agent-informant config show [--config PATH]
  agent-informant config init [--config PATH] [--force]`))}
func printSetupHelp(){fmt.Println(strings.TrimSpace(`Structural hook setup

Commands:
  agent-informant setup claude
  agent-informant setup codex
  agent-informant setup all

Setup merges Agent Informant into existing user hooks and installs SessionStart,
UserPromptSubmit, and PreToolUse checks. Use --executable PATH to pin a binary.`))}

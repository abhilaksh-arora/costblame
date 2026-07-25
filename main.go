// Command costblame attributes Claude Code token spend to git branches and
// projects.
//
// It reads the JSONL session logs Claude Code writes under ~/.claude/projects,
// deduplicates assistant turns by message id, buckets each turn into the branch
// active when it was written (recorded inline on the message), prices it, and
// reports per-branch / per-project / per-day spend.
//
// Usage:
//
//	costblame                                    a short intro (run 'sync' for real output)
//	costblame sync                               spend across every project
//	costblame [--repo DIR] [--all] [--json] [--by day|week] [--pricing FILE]
//	costblame serve [--repo DIR] [--pricing FILE] [--port N]
//	costblame init                               (alias of configure — set your plan)
//	costblame update                             update to the latest release
//	costblame uninstall                          (remove the binary + ~/.costblame config)
//	costblame version
//	costblame help
//
// All data stays on the local machine.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) == 1 {
		runIntro()
		return
	}
	switch os.Args[1] {
	case "serve":
		runServe(os.Args[2:])
		return
	case "configure", "init":
		runConfigure(os.Args[2:])
		return
	case "uninstall":
		runUninstall(os.Args[2:])
		return
	case "sync":
		runReport(append([]string{"--all"}, os.Args[2:]...))
		return
	case "update":
		runUpdate(os.Args[2:])
		return
	case "version", "--version", "-v":
		runVersion(os.Args[2:])
		return
	case "help", "--help", "-h":
		runHelp(os.Args[2:])
		return
	}
	runReport(os.Args[1:])
}

func runReport(args []string) {
	fs := flag.NewFlagSet("costblame", flag.ExitOnError)
	repo := fs.String("repo", ".", "path to the repo (defaults to cwd; ignored with --all)")
	all := fs.Bool("all", false, "aggregate every project under ~/.claude/projects")
	asJSON := fs.Bool("json", false, "emit the full report as JSON")
	by := fs.String("by", "", "instead of per-branch/project, bucket by time: day|week")
	raw := fs.Bool("raw", false, "show raw API-equivalent cost only, without the plan comparison")
	pricingPath := fs.String("pricing", "", "path to a pricing.json overriding the embedded defaults")
	fs.Parse(args)

	pt, src, err := LoadPricing(*pricingPath)
	if err != nil {
		fatal("loading pricing: %v", err)
	}

	var projects []ProjectData
	if *all {
		projects, err = CollectAll()
		if err != nil {
			fatal("scanning projects: %v", err)
		}
		if len(projects) == 0 {
			fatal("no project logs found under ~/.claude/projects")
		}
	} else {
		pd, err := CollectRepo(*repo)
		if err != nil {
			if os.IsNotExist(err) {
				dir, _ := ProjectDir(*repo)
				fatal("no Claude session folder for this repo\n  expected: %s\n  (has Claude Code been run in %s?  try --all)", dir, *repo)
			}
			fatal("reading sessions: %v", err)
		}
		projects = []ProjectData{pd}
	}

	rep := BuildReport(projects, pt, src)
	if len(rep.Projects) == 0 {
		fatal("no assistant turns found")
	}

	// Plan framing: never prompt for --json (it's a pipe consumer); prompt once
	// on first interactive table use.
	var cfg *Config
	if *asJSON {
		cfg, _ = LoadConfig()
	} else {
		cfg = ensureConfig()
	}
	attachPlan(&rep, cfg, *raw)

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(rep)
		return
	}

	if rep.Plan != nil {
		fmt.Printf("Your plan: %s (%s flat)\n", rep.Plan.Name, rep.Plan.PriceLabel)
		fmt.Printf("Your Claude usage at API rates: $%.2f (%.1fx your subscription cost)\n\n",
			rep.Plan.APICost, rep.Plan.Multiplier)
	}

	// When more than one AI provider is present, lead with the split so
	// Codex/Gemini spend isn't hidden inside the totals.
	if len(rep.Providers) > 1 {
		renderProviderTable(os.Stdout, rep)
		fmt.Println()
	}

	switch {
	case *by != "":
		if *by != "day" && *by != "week" {
			fatal("--by must be 'day' or 'week', got %q", *by)
		}
		renderTimeTable(os.Stdout, rep, *by)
	case *all:
		renderProjectTable(os.Stdout, rep)
	default:
		renderBranchTable(os.Stdout, rep.Projects[0])
		renderActivity(os.Stdout, rep.Projects[0])
	}

	fmt.Fprintf(os.Stderr, "\n%d project(s), %d session(s); pricing: %s\n",
		len(rep.Projects), rep.Sessions, src)
	if rep.Plan == nil {
		fmt.Fprintln(os.Stderr, "note: cost is estimated API list price. Run `costblame configure` to compare it against your plan.")
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "costblame: "+format+"\n", args...)
	os.Exit(1)
}

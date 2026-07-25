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
//	costblame sync [--all] [--repo DIR ...] [--json] [--by day|week] [--pricing FILE]
//	                                              bare: adds this repo to the synced set and
//	                                              shows the union of it; --repo (repeatable) for
//	                                              an explicit one-off list; --all for every repo
//	costblame remove [DIR]                       take a repo out of the synced set (default: cwd)
//	costblame synced                             list the synced set
//	costblame serve [--repo DIR ...] [--pricing FILE] [--port N]
//	                                              bare: same synced set as sync
//	costblame ignore [DIR]                       drop a repo from --all views (default: cwd)
//	costblame unignore [DIR]                     reverse ignore
//	costblame ignored                            list what's ignored
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
	"strings"
)

// repoList is a --repo flag that can be given more than once — e.g.
// `costblame sync --repo devTab --repo costblame` scopes to exactly those
// two, instead of one repo (a single --repo, or none) or every repo (--all).
type repoList []string

func (r *repoList) String() string { return strings.Join(*r, ",") }
func (r *repoList) Set(v string) error {
	*r = append(*r, v)
	return nil
}

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
	case "ignore":
		runIgnore(os.Args[2:])
		return
	case "unignore":
		runUnignore(os.Args[2:])
		return
	case "ignored":
		runIgnored(os.Args[2:])
		return
	case "synced":
		runSynced(os.Args[2:])
		return
	case "remove", "forget":
		runForget(os.Args[2:])
		return
	case "sync":
		// Same flags as the default report (--repo, --all, --json, ...); just
		// the explicit verb that actually reads logs, so a bare `costblame`
		// (no args at all) stays a no-op intro instead of silently scanning —
		// the same reason people type a bare `node`/`git` just to check it's
		// installed, not to run anything. Bare `sync` (no --all, no --repo)
		// adds the current repo to a persistent set and shows the union of
		// everything in it — a second `sync` somewhere else adds to that set
		// rather than replacing it. `costblame remove` takes a repo back out.
		runReport(os.Args[2:], true)
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
	runReport(os.Args[1:], false)
}

func runReport(args []string, sync bool) {
	fs := flag.NewFlagSet("costblame", flag.ExitOnError)
	var repos repoList
	fs.Var(&repos, "repo", "path to a repo (repeatable: --repo a --repo b); defaults to the synced set; ignored with --all")
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
	switch {
	case *all:
		projects, err = CollectAll()
		if err != nil {
			fatal("scanning projects: %v", err)
		}
		if len(projects) == 0 {
			fatal("no project logs found under ~/.claude/projects")
		}
	case len(repos) > 0:
		for _, r := range repos {
			pd, perr := CollectRepo(r)
			if perr != nil {
				if os.IsNotExist(perr) {
					dir, _ := ProjectDir(r)
					fatal("no Claude session folder for %q\n  expected: %s\n  (has Claude Code been run there?)", r, dir)
				}
				fatal("reading sessions for %q: %v", r, perr)
			}
			projects = append(projects, pd)
		}
	case sync:
		abs, aerr := absCwd()
		if aerr != nil {
			fatal("resolving current directory: %v", aerr)
		}
		list, aerr := AddSynced(abs)
		if aerr != nil {
			fatal("updating synced list: %v", aerr)
		}
		projects, err = projectsForSynced(list)
		if err != nil {
			fatal("reading sessions: %v", err)
		}
	default:
		pd, perr := CollectRepo(".")
		if perr != nil {
			if os.IsNotExist(perr) {
				dir, _ := ProjectDir(".")
				fatal("no Claude session folder for this repo\n  expected: %s\n  (has Claude Code been run here?  try --all)", dir)
			}
			fatal("reading sessions: %v", perr)
		}
		projects = []ProjectData{pd}
	}

	rep := BuildReport(projects, pt, src)
	if len(rep.Projects) == 0 {
		fatal("no assistant turns found")
	}

	// Plan framing: never prompt for --json (it's a pipe consumer); prompt once
	// per provider actually present in this report, on first interactive use.
	providerNames := make([]string, len(rep.Providers))
	for i, p := range rep.Providers {
		providerNames[i] = p.Provider
	}
	var cfg *Config
	if *asJSON {
		cfg, _ = LoadConfig()
	} else {
		cfg = ensureConfig(providerNames)
	}
	attachPlan(&rep, cfg, *raw)

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(rep)
		return
	}

	for _, pl := range rep.Plans {
		fmt.Printf("Your %s plan: %s (%s flat)\n", providerLabel[pl.Provider], pl.Name, pl.PriceLabel)
		fmt.Printf("Your %s usage at API rates: $%.2f (%.1fx your subscription cost)\n\n",
			providerLabel[pl.Provider], pl.APICost, pl.Multiplier)
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
	case *all || len(repos) > 1 || (sync && len(rep.Projects) > 1):
		renderProjectTable(os.Stdout, rep)
	default:
		renderBranchTable(os.Stdout, rep.Projects[0])
		renderActivity(os.Stdout, rep.Projects[0])
	}

	fmt.Fprintf(os.Stderr, "\n%d project(s), %d session(s); pricing: %s\n",
		len(rep.Projects), rep.Sessions, src)
	if len(rep.Plans) == 0 {
		fmt.Fprintln(os.Stderr, "note: cost is estimated API list price. Run `costblame configure` to compare it against your plan.")
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "costblame: "+format+"\n", args...)
	os.Exit(1)
}

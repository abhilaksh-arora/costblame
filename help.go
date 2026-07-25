package main

import "fmt"

// runIntro is what a bare `costblame` (no args at all) prints. It used to run
// the current-repo report — but that made it look like nothing happened for
// anyone who ran it in a repo other than the one they meant to check, since
// costblame never writes anything anywhere. A bare invocation is exactly what
// people type for other CLIs (node, git) just to check it's installed, not to
// run something — so it shouldn't scan logs either. `sync` is the explicit
// verb that actually reads them.
func runIntro() {
	fmt.Print(`costblame — attribute your AI coding token spend across Claude Code, Codex, and Gemini CLI.

  costblame sync        add this project and show everything synced so far
  costblame sync --all  spend across every project, synced or not
  costblame serve       open the local dashboard (same synced set as sync)
  costblame init        pick your plan (one-time)
  costblame update      update to the latest version
  costblame help        full command reference
`)
}

func runHelp(args []string) {
	fmt.Print(`costblame — attribute your AI coding token spend across Claude Code, Codex, and Gemini CLI.

COMMANDS
  costblame sync [flags]    add this repo to the synced set, show the union of it
                            (add --all for every project instead, synced or not)
  costblame remove [DIR]    take a repo out of the synced set (default: current directory)
  costblame synced          list the synced set
  costblame serve [flags]   open the local dashboard (same synced set as sync)
  costblame ignore [DIR]    drop a repo from --all views (default: current directory)
  costblame unignore [DIR]  reverse ignore
  costblame ignored         list what's currently ignored
  costblame init            pick your plan (alias of configure)
  costblame configure       same as init
  costblame update          update to the latest release
  costblame uninstall       remove the binary + local config (logs untouched)
  costblame version         print the installed version
  costblame help            this text

sync/serve with no --all and no --repo remember every repo you've ever run
them in bare, and always show the union of that set — run sync in a second
repo and it adds to the set, it doesn't replace it. costblame remove takes
one back out.

FLAGS (sync)
  --repo DIR       a repo to include, instead of the synced set; ignored with --all
                   repeatable — --repo a --repo b scopes to exactly those, no others,
                   and doesn't touch the synced set either way
  --all            aggregate every project under ~/.claude/projects, synced or not
  --json           emit the full report as JSON
  --by day|week    bucket spend by time instead of branch/project
  --raw            show API-equivalent cost only, no plan comparison
  --pricing FILE   override the built-in pricing table

FLAGS (serve)
  --repo DIR       a repo to scope the dashboard to, instead of the synced set
                   repeatable — --repo a --repo b shows exactly those, no others
  --all            scope the dashboard to every project, synced or not
  --pricing FILE   override the built-in pricing table
  --raw            drop the plan comparison
  --port N         localhost port (default 7777)

Ignoring a repo only drops it from --all views; explicitly syncing it or
naming it with --repo still shows it.

Nothing leaves your machine. Full docs: https://github.com/abhilaksh-arora/costblame
`)
}

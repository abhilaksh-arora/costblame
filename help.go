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

  costblame sync       spend for this project (start here)
  costblame sync --all spend across every project
  costblame serve      open the local dashboard
  costblame init       pick your plan (one-time)
  costblame update     update to the latest version
  costblame help       full command reference
`)
}

func runHelp(args []string) {
	fmt.Print(`costblame — attribute your AI coding token spend across Claude Code, Codex, and Gemini CLI.

COMMANDS
  costblame sync [flags]    spend for this project (add --all for every project)
  costblame serve [flags]   open the local dashboard
  costblame ignore [DIR]    drop a repo from --all views (default: current directory)
  costblame unignore [DIR]  reverse ignore
  costblame ignored         list what's currently ignored
  costblame init            pick your plan (alias of configure)
  costblame configure       same as init
  costblame update          update to the latest release
  costblame uninstall       remove the binary + local config (logs untouched)
  costblame version         print the installed version
  costblame help            this text

FLAGS (sync)
  --repo DIR       a repo to include (default: current directory; ignored with --all)
                   repeatable — --repo a --repo b scopes to exactly those, no others
  --all            aggregate every project under ~/.claude/projects
  --json           emit the full report as JSON
  --by day|week    bucket spend by time instead of branch/project
  --raw            show API-equivalent cost only, no plan comparison
  --pricing FILE   override the built-in pricing table

FLAGS (serve)
  --repo DIR       a repo to scope the dashboard to (default: current directory)
                   repeatable — --repo a --repo b shows exactly those, no others
  --all            scope the dashboard to every project
  --pricing FILE   override the built-in pricing table
  --raw            drop the plan comparison
  --port N         localhost port (default 7777)

Ignoring a repo only drops it from --all views; costblame sync --repo DIR
still shows an ignored repo directly if you ask for it by name.

Nothing leaves your machine. Full docs: https://github.com/abhilaksh-arora/costblame
`)
}

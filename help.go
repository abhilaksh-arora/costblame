package main

import "fmt"

// runIntro is what a bare `costblame` (no args at all) prints. It used to run
// the current-repo report — but that made it look like nothing happened for
// anyone who ran it in a repo other than the one they meant to check, since
// costblame never writes anything anywhere. `sync` is now the command that
// actually shows spend.
func runIntro() {
	fmt.Print(`costblame — attribute your AI coding token spend across Claude Code, Codex, and Gemini CLI.

  costblame sync      spend across every project (start here)
  costblame serve     open the local dashboard
  costblame init      pick your plan (one-time)
  costblame update    update to the latest version
  costblame help      full command reference
`)
}

func runHelp(args []string) {
	fmt.Print(`costblame — attribute your AI coding token spend across Claude Code, Codex, and Gemini CLI.

COMMANDS
  costblame sync            spend across every project you've used it in
  costblame [flags]         spend for one repo (default: the current directory)
  costblame serve [flags]   open the local dashboard
  costblame init            pick your plan (alias of configure)
  costblame configure       same as init
  costblame update          update to the latest release
  costblame uninstall       remove the binary + local config (logs untouched)
  costblame version         print the installed version
  costblame help            this text

FLAGS (report / sync)
  --repo DIR       path to the repo (default: current directory; ignored with --all)
  --all            aggregate every project under ~/.claude/projects (what sync uses)
  --json           emit the full report as JSON
  --by day|week    bucket spend by time instead of branch/project
  --raw            show API-equivalent cost only, no plan comparison
  --pricing FILE   override the built-in pricing table

FLAGS (serve)
  --repo DIR       scope the dashboard to one repo (default: current directory)
  --all            scope the dashboard to every project
  --pricing FILE   override the built-in pricing table
  --raw            drop the plan comparison
  --port N         localhost port (default 7777)

Nothing leaves your machine. Full docs: https://github.com/abhilaksh-arora/costblame
`)
}

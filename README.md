# costblame

[![Go 1.21+](https://img.shields.io/badge/Go-1.21%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Zero dependencies](https://img.shields.io/badge/dependencies-0-c17b24)](go.mod)
[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

See where your AI coding spend actually goes — **Claude Code, OpenAI Codex, and
Gemini CLI** — broken down by project, by provider, and (for Claude) by git
branch. Everything runs locally: it reads the session logs already sitting on
your machine, and nothing leaves it.

```
BRANCH   INPUT  OUTPUT  CACHE WR  CACHE RD  COST   SESSIONS  DURATION
feature  6966   21263   660502    3621888   $6.51  1         9m
main     1204    3891    120340    880210    $1.42  2         14m
TOTAL    8170   25154   780842   4502098    $7.93
```

The `COST` column is an **estimate at API list price** — useful for comparing
projects and spotting trends, not a literal invoice (most people run these
tools on a flat-rate subscription, not pay-as-you-go). See
[ARCHITECTURE.md](ARCHITECTURE.md) for exactly what it means and how it's
calculated.

## Install

Go stdlib only, no dependencies.

**Quickest — prebuilt binary via curl** (macOS/Linux; installs to `~/.local/bin`):

```sh
curl -fsSL https://raw.githubusercontent.com/abhilaksh-arora/costblame/main/install.sh | sh
```

**From a clone:**

```sh
git clone https://github.com/abhilaksh-arora/costblame.git
cd costblame
make install          # builds + installs to ~/.local/bin (must be on PATH)
```

Or straight from GitHub (installs to `$(go env GOPATH)/bin`):

```sh
go install github.com/abhilaksh-arora/costblame@latest
```

Remove it anytime with `costblame uninstall` (or `make uninstall`).

![costblame dashboard](assets/dashboard.png)

## Use

costblame doesn't write anything of its own except a small local memory of
*which* repos you care about — it always recomputes the numbers fresh from
the session logs Claude Code / Codex / Gemini already write locally. `sync`
is the command that shows that memory:

```sh
costblame init      # one-time: pick your plan, or skip
costblame sync        # add this repo to your synced set, show the union of it
costblame sync --all  # spend across every project, synced or not
costblame serve       # web dashboard → http://127.0.0.1:7777 (same synced set as sync)
```

Run `costblame sync` once per repo you want tracked — each run *adds* that
repo to the set, it doesn't replace what's already there, so syncing repo B
after repo A shows both together, not just B. `costblame remove` (or
`costblame synced` to see the current set) takes one back out:

```sh
costblame remove          # drop this repo from the synced set
costblame remove ~/work/api  # or name one explicitly
costblame synced             # list what's currently synced
```

Running `costblame` with no arguments prints a short reminder of all this
instead of a report. `--repo` repeats for a one-off, non-persistent list
that doesn't touch the synced set either way:

```sh
costblame sync --repo ~/work/api --repo ~/work/web   # just these two, right now
costblame serve --repo ~/work/api --repo ~/work/web  # same, in the dashboard
```

Or exclude a project from `--all` specifically (it can still be synced or
named directly) — run this once, from inside it:

```sh
costblame ignore      # this repo drops out of --all / sync --all / serve --all
costblame unignore    # undo
costblame ignored     # what's currently excluded
```

A few more commands and flags:

```sh
costblame sync --all --by day    # spend bucketed by day or week, across every project
costblame sync --json            # full report as JSON, for piping into other tools
costblame sync --pricing rates.json   # override the built-in pricing table
costblame serve --all            # dashboard across every project, not just this one
costblame update                 # update to the latest release
costblame version                # print the installed version
costblame help                   # full command + flag reference
```

## Dashboard

`costblame serve` opens a local, offline dashboard: cost vs. your plan,
spend by provider and by project, a daily trend, and (for Claude) code-impact
stats — lines changed, most-touched files, tool error rate. It binds to
`127.0.0.1` only and fetches nothing external.

## Learn more

[ARCHITECTURE.md](ARCHITECTURE.md) has the deeper reference: what the dollar
figure really means, how pricing and caching are calculated, accuracy caveats,
and how the code is laid out.

## License

[MIT](LICENSE)

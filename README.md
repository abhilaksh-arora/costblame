# costblame

Attribute your AI coding token spend (and estimated cost, and wall-clock time)
across **Claude Code, OpenAI Codex, and Gemini CLI** — per project, per provider,
and (for Claude) per git branch.

Each tool writes local session logs with per-turn token `usage`, a `timestamp`,
and the working directory. `costblame` reads all three, prices every turn, and
rolls the spend up by project and provider:

| Provider   | Log location                                  | Attribution        |
| ---------- | --------------------------------------------- | ------------------ |
| **Claude** | `~/.claude/projects/<encoded-path>/*.jsonl`   | per branch + project (logs carry `gitBranch`) |
| **Codex**  | `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl`| per project (cwd; no branch in logs) |
| **Gemini** | `~/.gemini/tmp/<projectHash>/**/*.jsonl`       | per project (cwd; no branch in logs) |

Codex and Gemini logs record no `gitBranch`, so their spend is attributed to the
project (recovered from the recorded working directory), not to a branch. Only
Claude carries a real per-branch breakdown. Any provider that isn't installed is
simply skipped.

## What the dollar figure means

The `COST` is **estimated API list price** — what your token usage *would* cost at
pay-as-you-go [API rates](https://claude.com/pricing#api). It is **not** an
invoice.

If you use Claude Code on a **subscription** (Pro at $20/mo, Max, Team), you pay a
**flat monthly rate**, not per token — so the number here will routinely exceed
what you actually pay. Read it as **value consumed / ROI** and for **relative**
comparison (which project or branch is the expensive one, how spend trends), not
as a bill. Cache-read tokens usually dominate the total; on a subscription those
sit inside your flat rate (subject to usage limits), not your dollars.

The figure *is* a real bill only if you drive the tool with a pay-as-you-go
**API key**. A Claude subscription offsets **only Claude** spend — Codex and
Gemini are billed on their own plans / keys, so `costblame` shows each provider's
cost separately and applies the plan multiplier to the Claude portion alone.

### Plan-aware framing (default)

So the number isn't mistaken for an invoice, costblame frames it against your
plan. On first run it asks once:

```sh
costblame configure   # pick your plan; stored in ~/.costblame/config.json
```

Menu: Pro ($20/mo), Max 5x ($100), Max 20x ($200), Team Standard ($25/seat),
Team Premium ($125/seat), API/pay-as-you-go, or Skip. After that, output leads
with the comparison instead of a bare dollar figure:

```
Your plan: Claude Pro ($20/mo flat)
Your Claude usage at API rates: $120.79 (6.0x your subscription cost)
```

When Codex or Gemini spend is present too, a per-provider split is printed above
the table (and shown as a segmented bar in the dashboard), so non-Claude cost is
never hidden inside the total.

- If you pick **API/pay-as-you-go** or **Skip**, no reframing is applied — the
  API-equivalent figure is your real cost (or you opted out), so it's shown as-is.
- The per-project / per-branch dollar figures are unchanged — still the only unit
  that compares spend across models. Only the headline gains a plan line.
- `--raw` on any command drops the plan line and shows the API-equivalent view.
- `--json` includes a `plan` block (`monthly_usd`, `api_cost`, `multiplier`) when
  a paid plan is configured; it never prompts.

The plan → flat-price mapping is a small hardcoded table (see `config.go`); update
it if Anthropic changes plan prices.

## Install

Go stdlib only, no dependencies. Once installed, run it from **any** repo.

**From a clone (recommended):**

```sh
git clone git@github.com:abhilaksh-arora/costblame.git
cd costblame
make install          # builds + installs to ~/.local/bin (must be on PATH)
```

Then, in any project:

```sh
costblame init        # one-time: pick your plan (alias of `configure`)
costblame serve       # dashboard → http://127.0.0.1:7777
costblame --all       # or a CLI table across every project
```

`make uninstall` removes it.

**Straight from GitHub** (installs to `$(go env GOPATH)/bin` — add that to your
PATH):

```sh
# private repo: allow the module and use SSH auth once
go env -w GOPRIVATE=github.com/abhilaksh-arora/*
git config --global url."git@github.com:".insteadOf "https://github.com/"

go install github.com/abhilaksh-arora/costblame@latest
```

**Just build a local binary:**

```sh
make build            # → ./costblame   (or: go build -o costblame .)
```

**Cross-compile release zips** (macOS/Linux/Windows) for sharing:

```sh
make dist             # → dist/*.zip
```

## Usage

```sh
costblame                        # branches of the repo in the current dir
costblame --repo /path/to/repo   # a specific repo
costblame --all                  # every project under ~/.claude/projects
costblame --all --by day         # spend bucketed by day (or: --by week)
costblame --json                 # full report as JSON (pipeable)
costblame --pricing rates.json   # override pricing (see below)
costblame serve                  # local web dashboard (see below)
```

### Default — per-branch, one repo

```
BRANCH   INPUT  OUTPUT  CACHE WR  CACHE RD  COST   SESSIONS  DURATION
feature  6966   21263   660502    3621888   $6.51  1         9m
main     1204    3891    120340    880210    $1.42  2         14m
TOTAL    8170   25154   780842   4502098    $7.93
```

Columns: input / output / cache-write / cache-read tokens, estimated USD cost,
distinct sessions touched, and the span from first to last message.

### `--all` — per-project rollup

Scans every provider, recovers each real repo path from the `cwd` recorded in
the logs (accurate even when Claude's folder-name encoding is ambiguous), and
rolls up per project, sorted by cost. When more than one provider is present the
per-provider split is printed first:

```
PROVIDER  INPUT     OUTPUT   CACHE RD   COST     SHARE  SESSIONS
claude    172360    2098969  969682961  $424.04  80%    13
codex     13749214  1691031  300611840  $101.86  19%    23
gemini    1601178   59948    15713866   $5.30    1%     2
TOTAL                                   $531.20         37

PROJECT                       INPUT  OUTPUT  CACHE WR  CACHE RD   COST     SESSIONS
/Users/me/workspace/devTab    687070 617777  3776750   394482319  $149.83  3
/Users/me/workspace/vendi     10701  268961  925811    74016103   $48.41   1
...
```

### `--by day|week` — timeline

Buckets spend across all included projects by day or ISO week — good for
"how has my Claude spend trended?"

### `--json` — machine-readable

Emits the complete report (projects → branches → daily buckets, with per-TTL
cache token splits) to stdout. This is the integration surface: pipe it into
`jq`, a spreadsheet, or a future team collector. With `--json`, only JSON goes to
stdout.

For the table modes, a one-line diagnostic (projects, sessions, pricing source)
is printed to **stderr**, so `costblame > report.txt` captures just the table.

## Dashboard

```sh
costblame serve            # all projects, http://127.0.0.1:7777
costblame serve --repo .   # scope to one repo
costblame serve --port 8080
```

A local, offline web dashboard: a plan-vs-usage hero, a **spend-by-provider**
segmented bar (Claude / Codex / Gemini) with legend, summary tiles, a
cost-by-project bar list, and click-to-expand rows showing the per-provider and
per-branch split plus a daily cost strip. It **binds to localhost only** and
serves nothing but its own embedded HTML — no data leaves the machine, no
external assets are fetched. The report is rebuilt on every request, so
**Refresh** always reflects current logs.

### Notes on accuracy

- **Deduplication.** Claude writes one assistant turn as several lines (one per
  content block — thinking, text, tool_use) that repeat the same `usage`, so
  costblame keys on `message.id` and counts each turn once. Codex reports a
  running cumulative total per session, so the final total is taken (one Event
  per session). Gemini writes one line per turn, deduplicated by its `id`.
- **Token normalization.** Providers report usage differently: Codex and Gemini
  fold cached tokens into `input` (split back out to a cache-read line) and Codex
  folds reasoning into `output`, while Gemini reports `thoughts` separately
  (added to output). All three normalize into the same input / output / cache
  fields before pricing.
- **Cache dominates.** In real logs, cache-read (and, for Claude, cache-write)
  tokens vastly outweigh raw input tokens, so cost is priced with cache in mind.
- **Branch source.** For Claude the branch comes from the `gitBranch` field on
  every message — no git history is read; a detached HEAD or non-git dir shows as
  `HEAD`, an absent field as `(unknown)`. Codex and Gemini record no branch, so
  their spend shows under `(n/a)` and is attributed by project instead.

## Pricing

Rates live in a small table. Resolution order:

1. `--pricing <path>` if given
2. `pricing.json` sitting next to the `costblame` binary
3. the table **embedded in the binary** at build time (standard published rates)

### `pricing.json` format

USD **per 1,000,000 tokens**. Models are matched by **longest id prefix**, so one
entry covers every version in a family (`claude-opus-4-8`, `-4-7`, `-4-6`, …).

```json
{
  "models": [
    { "prefix": "claude-opus-",   "input": 5.0,  "output": 25.0, "cache_write": 6.25, "cache_write_1h": 10.0, "cache_read": 0.5 },
    { "prefix": "gpt-5.3-codex",  "input": 1.75, "output": 14.0, "cache_write": 0.0,  "cache_write_1h": 0.0,  "cache_read": 0.175 },
    { "prefix": "gemini-3-pro",   "input": 2.0,  "output": 12.0, "cache_write": 0.0,  "cache_write_1h": 0.0,  "cache_read": 0.2 }
  ]
}
```

OpenAI and Google models cache automatically with no separate cache-*write*
charge, so their `cache_write` / `cache_write_1h` are `0` and only `cache_read`
(the cached-input rate) applies. Because costblame tracks the Codex CLI, the
generic `gpt-5` fallback uses Codex rates; explicit `gpt-5.4` / `gpt-5.5` entries
keep their standard rates via a longer prefix.

`cache_write` is the 5-minute-TTL rate (~1.25× input); `cache_write_1h` is the
1-hour-TTL rate (~2× input). `costblame` reads the per-TTL breakdown that Claude
Code records (`ephemeral_5m_input_tokens` / `ephemeral_1h_input_tokens`) and
prices each portion at its own rate, so extended (1-hour) caching is billed
correctly rather than assumed. A `pricing.json` that omits `cache_write_1h`
(older format) has it derived as `1.6 × cache_write`. The `CACHE WR` column shows
the combined 5m + 1h token count.

A model that matches no prefix is counted as `$0` and warned about on stderr.

### Keeping rates current

The embedded defaults are standard published rates, verified 2026-07-12. When a
provider changes pricing:

- **Refresh from source** (all render rates with JavaScript, so copy by hand):
  - Anthropic — <https://claude.com/pricing#api>
  - OpenAI — <https://developers.openai.com/api/docs/pricing>
  - Google — <https://ai.google.dev/gemini-api/docs/pricing>
- **Update without rebuilding:** drop an updated `pricing.json` next to the
  binary, or point `--pricing` at it. No recompile needed.
- **Update the defaults:** edit `pricing.json` in this repo and rebuild — it is
  embedded via `//go:embed`.

Cache rates follow Anthropic's model: `cache_read ≈ 0.1×` input, `cache_write ≈
1.25×` input (5-minute TTL), and `cache_write_1h ≈ 2×` input (1-hour TTL).
Promotional/intro rates (e.g. a temporary Sonnet discount) are **not** baked into
the defaults — supply a `pricing.json` if you want them.

## Layout

Parsing, aggregation, and presentation are separate, so the CLI, JSON, and
dashboard all share one aggregation path:

| File             | Responsibility                                              |
| ---------------- | ----------------------------------------------------------- |
| `paths.go`       | repo path → `~/.claude/projects` folder; list `.jsonl`      |
| `sessions.go`    | parse Claude JSONL → deduplicated `[]Event`                 |
| `codex.go`       | parse Codex `rollout-*.jsonl` → `[]Event` (per session)     |
| `gemini.go`      | parse Gemini chat `*.jsonl` → `[]Event` (per turn)          |
| `collect.go`     | merge all providers, group by repo (`--repo` or `--all`)    |
| `model.go`       | the shared `Report` / `ProjectReport` / `BranchStat` types  |
| `aggregate.go`   | `[]ProjectData` → `Report` (branches, projects, daily)      |
| `pricing.go`     | embedded/override pricing table + per-TTL cost calc         |
| `report.go`      | branch / project / time tables via `text/tabwriter`         |
| `serve.go`       | `serve` subcommand: localhost HTTP + embedded dashboard     |
| `dashboard.html` | the offline single-page dashboard (embedded via `go:embed`) |
| `main.go`        | flag parsing + subcommand dispatch                          |

Everything is Go stdlib — `net/http`, `embed`, `encoding/json`, `text/tabwriter`,
etc. No third-party dependencies.
```

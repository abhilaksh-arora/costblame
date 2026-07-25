# How costblame works

The deeper reference for pricing internals, accuracy caveats, and code layout.
For install/usage, see [README.md](README.md).

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

## Notes on accuracy

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
- **Code impact.** Edit/Write/MultiEdit/NotebookEdit tool calls (Claude only) are
  parsed for added/removed line counts via a line-count diff, not a true LCS
  diff — good enough for cost-per-edit / cost-per-100-lines and a most-touched-
  files view, not for rendering an actual patch.

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

The embedded defaults are standard published rates — Anthropic rates verified
2026-07-25, OpenAI/Google 2026-07-12. When a provider changes pricing:

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
| `activity.go`    | parse Claude edit/correction/tool-call activity             |
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

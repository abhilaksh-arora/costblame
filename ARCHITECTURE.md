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
**API key**. A subscription offsets **only the provider it's for** — a Claude
plan never offsets Codex or Gemini spend, since they're billed separately.
`costblame` shows each provider's cost on its own, and (per below) can frame
each one against its own plan independently.

### Plan-aware framing (default), per provider

So the number isn't mistaken for an invoice, costblame frames it against your
plan — one plan per provider, since each is billed separately. On first run it
asks once per provider that actually has spend in that run:

```sh
costblame configure   # pick a plan for Claude, Codex, Gemini — any or all
```

`configure` is a picker loop: choose a provider, pick its plan, land back on
the picker to do another or hit Done. Re-run it anytime to add a provider you
didn't have before or change one you already set. Each provider's config is
stored under its own key in `~/.costblame/config.json`
(`{"plans": {"claude": {...}, "codex": {...}}}`); a config file from before
multi-provider support (a bare `{"plan": "pro", ...}`) is auto-migrated into
`plans.claude` the first time it's read.

The first-run auto-prompt (triggered by `sync`/`serve`, not `configure`) asks
about one provider at a time, labeled "provider N of M" when there's more than
one new provider to set up, with a `[0] Skip this and all remaining
providers` shortcut — picking it saves every not-yet-asked provider as Skip
too, so it isn't asked again next run.

Catalogs (verified 2026-07-25 — check each provider's own pricing page if a
number looks stale):

| Provider | Plans |
|---|---|
| Claude | Pro ($20/mo), Max 5x ($100), Max 20x ($200), Team Standard ($25/seat), Team Premium ($125/seat) |
| Codex (ChatGPT) | Plus ($20/mo), Pro ($200/mo), Team ($25/seat) |
| Gemini | Google AI Pro ($19.99/mo), Google AI Ultra ($249.99/mo), Code Assist Standard ($19/seat), Code Assist Enterprise ($45/seat) |

Every catalog also offers **API / pay-as-you-go** and **Skip** — no reframing
for that provider, its API-equivalent figure is shown as-is. After
configuring, output leads with one comparison line per configured provider
that has spend, instead of a bare dollar figure:

```
Your Claude plan: Claude Pro ($20/mo flat)
Your Claude usage at API rates: $120.79 (6.0x your subscription cost)

Your Codex (ChatGPT) plan: ChatGPT Plus ($20/mo flat)
Your Codex (ChatGPT) usage at API rates: $1.14 (0.1x your subscription cost)
```

A per-provider split is also printed above the table whenever more than one
provider has spend (and shown as a segmented bar in the dashboard), so
non-Claude cost is never hidden inside the total.

- The per-project / per-branch dollar figures are unchanged — still the only
  unit that compares spend across models. Only the headline gains plan lines.
- `--raw` on any command drops all plan lines and shows the plain
  API-equivalent view.
- `--json` includes a `plans` array (`provider`, `monthly_usd`, `api_cost`,
  `multiplier` per entry) for whichever providers are configured and have
  spend; it never prompts.

The plan → flat-price catalogs are a small hardcoded table per provider (see
`config.go`, `planCatalogs`); update them if a provider changes plan prices.

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

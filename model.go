package main

import "time"

// Totals is a token + cost accumulator shared by branches, projects, days.
type Totals struct {
	Input         int     `json:"input"`
	Output        int     `json:"output"`
	CacheCreate   int     `json:"cache_create"`    // total cache-write tokens (5m + 1h)
	CacheCreate1h int     `json:"cache_create_1h"` // subset written with 1h TTL
	CacheRead     int     `json:"cache_read"`
	Cost          float64 `json:"cost"`
}

func (t *Totals) add(e Event, cost float64) {
	t.Input += e.Input
	t.Output += e.Output
	t.CacheCreate += e.CacheCreate
	t.CacheCreate1h += e.CacheCreate1h
	t.CacheRead += e.CacheRead
	t.Cost += cost
}

func (t *Totals) addTotals(s Totals) {
	t.Input += s.Input
	t.Output += s.Output
	t.CacheCreate += s.CacheCreate
	t.CacheCreate1h += s.CacheCreate1h
	t.CacheRead += s.CacheRead
	t.Cost += s.Cost
}

// BranchStat is the spend attributed to one branch within a project.
type BranchStat struct {
	Branch   string        `json:"branch"`
	Totals   Totals        `json:"totals"`
	Sessions int           `json:"sessions"`
	First    time.Time     `json:"first"`
	Last     time.Time     `json:"last"`
	Activity ActivityStats `json:"activity"`
}

// ActivityStats is code-impact and workflow signal derived from Claude's
// Edit/MultiEdit/Write/NotebookEdit tool calls and user turns — Claude-only,
// since Codex and Gemini logs don't carry the same tool payloads. ReworkLoops
// and Corrections are rolled up at project/report level only (0 on a
// BranchStat), since they're session-scoped signals a per-branch split would
// just be double-counting across an already-small sample.
type ActivityStats struct {
	Edits           int     `json:"edits"`
	LinesAdded      int     `json:"lines_added"`
	LinesRemoved    int     `json:"lines_removed"`
	ChangedLines    int     `json:"changed_lines"`
	CostPerEdit     float64 `json:"cost_per_edit,omitempty"`
	CostPer100Lines float64 `json:"cost_per_100_lines,omitempty"`
	ReworkLoops     int     `json:"rework_loops"`
	Corrections     int     `json:"corrections"`
	ToolCalls       int     `json:"tool_calls"`
	ToolErrors      int     `json:"tool_errors"`
	ToolErrorRate   float64 `json:"tool_error_rate,omitempty"`
}

func (a *ActivityStats) addLines(added, removed int) {
	a.Edits++
	a.LinesAdded += added
	a.LinesRemoved += removed
	a.ChangedLines += added + removed
}

// finalize derives the cost- and rate-based fields once the raw counts (and
// the cost they should be priced against) are known.
func (a *ActivityStats) finalize(cost float64) {
	if a.Edits > 0 {
		a.CostPerEdit = cost / float64(a.Edits)
	}
	if a.ChangedLines > 0 {
		a.CostPer100Lines = cost / float64(a.ChangedLines) * 100
	}
	if a.ToolCalls > 0 {
		a.ToolErrorRate = float64(a.ToolErrors) / float64(a.ToolCalls)
	}
}

// FileStat is one file's edit activity within a project, for a "most-touched
// files" view.
type FileStat struct {
	Path         string `json:"path"`
	Edits        int    `json:"edits"`
	LinesAdded   int    `json:"lines_added"`
	LinesRemoved int    `json:"lines_removed"`
	Sessions     int    `json:"sessions"`
}

// ToolStat is call/error counts for one tool name (Edit, Bash, Read, ...).
type ToolStat struct {
	Name   string `json:"name"`
	Calls  int    `json:"calls"`
	Errors int    `json:"errors"`
}

// DayBucket is one day's spend (UTC), for timelines.
type DayBucket struct {
	Date   string `json:"date"` // YYYY-MM-DD
	Totals Totals `json:"totals"`
}

// TimelinePoint is one day's total cost across all projects, split by provider,
// for the dashboard's spend-over-time chart.
type TimelinePoint struct {
	Date       string             `json:"date"` // YYYY-MM-DD
	Total      float64            `json:"total"`
	ByProvider map[string]float64 `json:"by_provider"`
}

// ProviderStat is spend attributed to one AI provider (claude/codex/gemini).
type ProviderStat struct {
	Provider string `json:"provider"`
	Totals   Totals `json:"totals"`
	Sessions int    `json:"sessions"`
}

// ProjectReport is the spend for one repo (project folder).
type ProjectReport struct {
	Project   string         `json:"project"` // repo path
	Folder    string         `json:"folder"`  // ~/.claude/projects folder name
	Totals    Totals         `json:"totals"`
	Sessions  int            `json:"sessions"`
	First     time.Time      `json:"first"`
	Last      time.Time      `json:"last"`
	Providers []ProviderStat `json:"providers"` // per-provider split, cost desc
	Branches  []BranchStat   `json:"branches"`  // per-branch (Claude only carries a real branch)
	Daily     []DayBucket    `json:"daily"`
	Activity  ActivityStats  `json:"activity"`        // Claude-only code-impact + workflow signals
	Files     []FileStat     `json:"files,omitempty"` // most-touched files, top 15, edits desc
	Tools     []ToolStat     `json:"tools,omitempty"` // tool call/error counts, calls desc
}

// PlanInfo frames one provider's API-equivalent cost against its flat
// subscription price. One of these exists per provider that both has spend
// in the report and a plan configured for it (see Report.Plans).
type PlanInfo struct {
	Provider   string  `json:"provider"` // "claude" | "codex" | "gemini"
	Name       string  `json:"name"`
	PriceLabel string  `json:"price_label"`
	MonthlyUSD float64 `json:"monthly_usd"`
	APICost    float64 `json:"api_cost"`   // = that provider's Totals.Cost
	Multiplier float64 `json:"multiplier"` // APICost / MonthlyUSD
}

// Report is the top-level aggregate consumed by the table, JSON, and dashboard.
type Report struct {
	GeneratedAt time.Time       `json:"generated_at"`
	Pricing     string          `json:"pricing"`
	Plans       []PlanInfo      `json:"plans,omitempty"` // one per configured provider with spend here, cost desc
	Totals      Totals          `json:"totals"`
	Sessions    int             `json:"sessions"`
	Providers   []ProviderStat  `json:"providers"`          // spend split by provider, cost desc
	Timeline    []TimelinePoint `json:"timeline,omitempty"` // daily cost per provider, date asc
	Projects    []ProjectReport `json:"projects"`
	Activity    ActivityStats   `json:"activity"`        // Claude-only, summed across all included projects
	Files       []FileStat      `json:"files,omitempty"` // most-touched files across all included projects, top 15
	Tools       []ToolStat      `json:"tools,omitempty"` // tool call/error counts across all included projects
}

// providerCost returns the total cost attributed to one provider.
func (r *Report) providerCost(provider string) float64 {
	for _, p := range r.Providers {
		if p.Provider == provider {
			return p.Totals.Cost
		}
	}
	return 0
}

// attachPlan sets rep.Plans (one entry per provider that both has spend in
// this report and a configured plan) unless raw framing was requested. Each
// provider's plan offsets only that provider's own spend — a Claude
// subscription never offsets Codex or Gemini cost, since they're billed
// separately. Providers are considered in rep.Providers order (cost desc), so
// rep.Plans comes out cost-sorted too. Changes no token or cost math.
func attachPlan(rep *Report, cfg *Config, raw bool) {
	if raw || cfg == nil {
		return
	}
	for _, ps := range rep.Providers {
		pp, ok := cfg.Plans[ps.Provider]
		if !ok || pp.MonthlyUSD <= 0 {
			continue
		}
		cost := ps.Totals.Cost
		rep.Plans = append(rep.Plans, PlanInfo{
			Provider:   ps.Provider,
			Name:       pp.PlanName,
			PriceLabel: pp.PriceLabel,
			MonthlyUSD: pp.MonthlyUSD,
			APICost:    cost,
			Multiplier: cost / pp.MonthlyUSD,
		})
	}
}

// PlanFor returns the PlanInfo for one provider, or nil if none is configured
// / that provider has no spend in this report.
func (r *Report) PlanFor(provider string) *PlanInfo {
	for i := range r.Plans {
		if r.Plans[i].Provider == provider {
			return &r.Plans[i]
		}
	}
	return nil
}

// extend widens [first,last] to include t (ignoring the zero time).
func extend(first, last *time.Time, t time.Time) {
	if t.IsZero() {
		return
	}
	if first.IsZero() || t.Before(*first) {
		*first = t
	}
	if t.After(*last) {
		*last = t
	}
}

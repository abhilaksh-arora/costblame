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
	Branch   string    `json:"branch"`
	Totals   Totals    `json:"totals"`
	Sessions int       `json:"sessions"`
	First    time.Time `json:"first"`
	Last     time.Time `json:"last"`
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
}

// PlanInfo frames the total API-equivalent cost against a flat subscription
// price. Present only when a paid plan is configured (nil for API/skip/--raw).
type PlanInfo struct {
	Name       string  `json:"name"`
	PriceLabel string  `json:"price_label"`
	MonthlyUSD float64 `json:"monthly_usd"`
	APICost    float64 `json:"api_cost"`   // = Report.Totals.Cost
	Multiplier float64 `json:"multiplier"` // APICost / MonthlyUSD
}

// Report is the top-level aggregate consumed by the table, JSON, and dashboard.
type Report struct {
	GeneratedAt time.Time       `json:"generated_at"`
	Pricing     string          `json:"pricing"`
	Plan        *PlanInfo       `json:"plan,omitempty"`
	Totals      Totals          `json:"totals"`
	Sessions    int             `json:"sessions"`
	Providers   []ProviderStat  `json:"providers"`          // spend split by provider, cost desc
	Timeline    []TimelinePoint `json:"timeline,omitempty"` // daily cost per provider, date asc
	Projects    []ProjectReport `json:"projects"`
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

// attachPlan sets rep.Plan when a paid subscription is configured and raw
// framing was not requested. The plan (a Claude subscription) offsets only
// Claude-provider spend, so the multiplier is computed against the Claude
// portion — Codex/Gemini are billed separately and shown as their own cost. It
// changes no token or cost math.
func attachPlan(rep *Report, cfg *Config, raw bool) {
	if raw || cfg == nil || cfg.MonthlyUSD <= 0 {
		return
	}
	claudeCost := rep.providerCost("claude")
	rep.Plan = &PlanInfo{
		Name:       cfg.PlanName,
		PriceLabel: cfg.PriceLabel,
		MonthlyUSD: cfg.MonthlyUSD,
		APICost:    claudeCost,
		Multiplier: claudeCost / cfg.MonthlyUSD,
	}
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

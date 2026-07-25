package main

import (
	"fmt"
	"os"
	"sort"
	"time"
)

// BuildReport aggregates collected projects into a Report, priced by pt.
// Models with no matching rate are reported once each to stderr.
func BuildReport(projects []ProjectData, pt *PricingTable, pricingSrc string) Report {
	rep := Report{GeneratedAt: time.Now(), Pricing: pricingSrc}
	unpriced := map[string]bool{}
	allSessions := map[string]bool{}
	provTotals := map[string]*Totals{}
	provSessions := map[string]map[string]bool{}
	timeline := map[string]map[string]float64{} // date -> provider -> cost

	type gfacc struct {
		FileStat
		sessions map[string]bool
	}
	globalFiles := map[string]*gfacc{}
	globalTools := map[string]*ToolStat{}

	for _, pd := range projects {
		pr := aggregateProject(pd, pt, unpriced, timeline)
		if len(pr.Branches) == 0 {
			continue // no assistant turns
		}
		rep.Projects = append(rep.Projects, pr)
		rep.Totals.addTotals(pr.Totals)
		rep.Activity.Edits += pr.Activity.Edits
		rep.Activity.LinesAdded += pr.Activity.LinesAdded
		rep.Activity.LinesRemoved += pr.Activity.LinesRemoved
		rep.Activity.ChangedLines += pr.Activity.ChangedLines
		rep.Activity.ReworkLoops += pr.Activity.ReworkLoops
		rep.Activity.Corrections += pr.Activity.Corrections
		rep.Activity.ToolCalls += pr.Activity.ToolCalls
		rep.Activity.ToolErrors += pr.Activity.ToolErrors
		// Rebuilt from the raw per-project edits/calls (not from pr.Files/pr.Tools,
		// which are already truncated to their project-level top N) so the
		// cross-project top list isn't skewed by an early truncation.
		for _, e := range pd.Edits {
			f := globalFiles[e.FilePath]
			if f == nil {
				f = &gfacc{sessions: map[string]bool{}}
				f.Path = e.FilePath
				globalFiles[e.FilePath] = f
			}
			f.Edits++
			f.LinesAdded += e.Added
			f.LinesRemoved += e.Removed
			if e.SessionID != "" {
				f.sessions[e.SessionID] = true
			}
		}
		for _, c := range pd.Calls {
			t := globalTools[c.Name]
			if t == nil {
				t = &ToolStat{Name: c.Name}
				globalTools[c.Name] = t
			}
			t.Calls++
			if c.IsError {
				t.Errors++
			}
		}
		for _, e := range pd.Events {
			if e.SessionID != "" {
				allSessions[e.SessionID] = true
			}
			p := e.Provider
			if p == "" {
				p = "unknown"
			}
			if provSessions[p] == nil {
				provSessions[p] = map[string]bool{}
			}
			if e.SessionID != "" {
				provSessions[p][e.SessionID] = true
			}
		}
		for _, ps := range pr.Providers {
			t := provTotals[ps.Provider]
			if t == nil {
				t = &Totals{}
				provTotals[ps.Provider] = t
			}
			t.addTotals(ps.Totals)
		}
	}
	rep.Sessions = len(allSessions)

	for p, t := range provTotals {
		rep.Providers = append(rep.Providers, ProviderStat{
			Provider: p, Totals: *t, Sessions: len(provSessions[p]),
		})
	}
	sort.SliceStable(rep.Providers, func(i, j int) bool {
		return rep.Providers[i].Totals.Cost > rep.Providers[j].Totals.Cost
	})

	dates := make([]string, 0, len(timeline))
	for d := range timeline {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	for _, d := range dates {
		var tot float64
		for _, c := range timeline[d] {
			tot += c
		}
		rep.Timeline = append(rep.Timeline, TimelinePoint{
			Date: d, Total: tot, ByProvider: timeline[d],
		})
	}

	sort.SliceStable(rep.Projects, func(i, j int) bool {
		return rep.Projects[i].Totals.Cost > rep.Projects[j].Totals.Cost
	})

	// Edits/tool calls are Claude-only, so price them against Claude's cost
	// alone — mixing in Codex/Gemini cost from the same window would skew
	// cost-per-edit for anyone running more than one provider.
	rep.Activity.finalize(rep.providerCost("claude"))
	for _, f := range globalFiles {
		f.Sessions = len(f.sessions)
		rep.Files = append(rep.Files, f.FileStat)
	}
	sort.SliceStable(rep.Files, func(i, j int) bool {
		ci, cj := rep.Files[i].LinesAdded+rep.Files[i].LinesRemoved, rep.Files[j].LinesAdded+rep.Files[j].LinesRemoved
		if ci != cj {
			return ci > cj
		}
		return rep.Files[i].Edits > rep.Files[j].Edits
	})
	if len(rep.Files) > 15 {
		rep.Files = rep.Files[:15]
	}
	for _, t := range globalTools {
		rep.Tools = append(rep.Tools, *t)
	}
	sort.SliceStable(rep.Tools, func(i, j int) bool { return rep.Tools[i].Calls > rep.Tools[j].Calls })
	if len(rep.Tools) > 12 {
		rep.Tools = rep.Tools[:12]
	}

	for m := range unpriced {
		fmt.Fprintf(os.Stderr, "warning: no pricing rate for model %q; counted as $0\n", m)
	}
	return rep
}

func aggregateProject(pd ProjectData, pt *PricingTable, unpriced map[string]bool, timeline map[string]map[string]float64) ProjectReport {
	type bacc struct {
		BranchStat
		sessions map[string]bool
	}
	type pacc struct {
		Totals
		sessions map[string]bool
	}
	type facc struct {
		FileStat
		sessions map[string]bool
	}
	branches := map[string]*bacc{}
	providers := map[string]*pacc{}
	daily := map[string]*DayBucket{}
	files := map[string]*facc{}
	tools := map[string]*ToolStat{}
	projSessions := map[string]bool{}
	pr := ProjectReport{Folder: pd.Folder, Project: pd.RepoPath}

	getBranch := func(name string) *bacc {
		b := branches[name]
		if b == nil {
			b = &bacc{sessions: map[string]bool{}}
			b.Branch = name
			branches[name] = b
		}
		return b
	}

	for _, e := range pd.Events {
		write5m := e.CacheCreate - e.CacheCreate1h
		cost, found := pt.Cost(e.Model, e.Input, e.Output, write5m, e.CacheCreate1h, e.CacheRead)
		if !found && e.Model != "" && !isNonBillable(e.Model) {
			unpriced[e.Model] = true
		}

		prov := e.Provider
		if prov == "" {
			prov = "unknown"
		}
		pa := providers[prov]
		if pa == nil {
			pa = &pacc{sessions: map[string]bool{}}
			providers[prov] = pa
		}
		pa.Totals.add(e, cost)
		if e.SessionID != "" {
			pa.sessions[e.SessionID] = true
		}

		b := branches[e.Branch]
		if b == nil {
			b = &bacc{sessions: map[string]bool{}}
			b.Branch = e.Branch
			branches[e.Branch] = b
		}
		b.Totals.add(e, cost)
		if e.SessionID != "" {
			b.sessions[e.SessionID] = true
		}
		extend(&b.First, &b.Last, e.Timestamp)

		pr.Totals.add(e, cost)
		if e.SessionID != "" {
			projSessions[e.SessionID] = true
		}
		extend(&pr.First, &pr.Last, e.Timestamp)

		if !e.Timestamp.IsZero() {
			day := e.Timestamp.UTC().Format("2006-01-02")
			d := daily[day]
			if d == nil {
				d = &DayBucket{Date: day}
				daily[day] = d
			}
			d.Totals.add(e, cost)

			if timeline != nil {
				if timeline[day] == nil {
					timeline[day] = map[string]float64{}
				}
				timeline[day][prov] += cost
			}
		}
	}

	// sessionFileEdits tracks, per session, how many times each file was
	// edited — the input to the rework-loop count below.
	sessionFileEdits := map[string]map[string]int{}
	for _, e := range pd.Edits {
		pr.Activity.addLines(e.Added, e.Removed)
		getBranch(e.Branch).Activity.addLines(e.Added, e.Removed)

		if e.FilePath != "" {
			f := files[e.FilePath]
			if f == nil {
				f = &facc{sessions: map[string]bool{}}
				f.Path = e.FilePath
				files[e.FilePath] = f
			}
			f.Edits++
			f.LinesAdded += e.Added
			f.LinesRemoved += e.Removed
			if e.SessionID != "" {
				f.sessions[e.SessionID] = true
			}
		}
		if e.SessionID != "" && e.FilePath != "" {
			if sessionFileEdits[e.SessionID] == nil {
				sessionFileEdits[e.SessionID] = map[string]int{}
			}
			sessionFileEdits[e.SessionID][e.FilePath]++
		}
	}
	for _, filesTouched := range sessionFileEdits {
		for _, n := range filesTouched {
			if n > 1 {
				pr.Activity.ReworkLoops += n - 1
			}
		}
	}
	pr.Activity.Corrections = len(pd.Corrections)

	for _, c := range pd.Calls {
		pr.Activity.ToolCalls++
		b := getBranch(c.Branch)
		b.Activity.ToolCalls++
		if c.IsError {
			pr.Activity.ToolErrors++
			b.Activity.ToolErrors++
		}
		t := tools[c.Name]
		if t == nil {
			t = &ToolStat{Name: c.Name}
			tools[c.Name] = t
		}
		t.Calls++
		if c.IsError {
			t.Errors++
		}
	}

	pr.Sessions = len(projSessions)
	for _, b := range branches {
		b.Sessions = len(b.sessions)
		b.Activity.finalize(b.Totals.Cost)
		pr.Branches = append(pr.Branches, b.BranchStat)
	}
	sort.SliceStable(pr.Branches, func(i, j int) bool {
		return pr.Branches[i].Totals.Cost > pr.Branches[j].Totals.Cost
	})
	for name, pa := range providers {
		pr.Providers = append(pr.Providers, ProviderStat{
			Provider: name, Totals: pa.Totals, Sessions: len(pa.sessions),
		})
	}
	sort.SliceStable(pr.Providers, func(i, j int) bool {
		return pr.Providers[i].Totals.Cost > pr.Providers[j].Totals.Cost
	})
	for _, d := range daily {
		pr.Daily = append(pr.Daily, *d)
	}
	sort.SliceStable(pr.Daily, func(i, j int) bool { return pr.Daily[i].Date < pr.Daily[j].Date })

	// Edits/tool calls are Claude-only, so price them against Claude's cost
	// alone, not pr.Totals.Cost, which may also include Codex/Gemini spend on
	// the same project.
	claudeCost := 0.0
	if cp, ok := providers["claude"]; ok {
		claudeCost = cp.Totals.Cost
	}
	pr.Activity.finalize(claudeCost)

	for _, f := range files {
		f.Sessions = len(f.sessions)
		pr.Files = append(pr.Files, f.FileStat)
	}
	sort.SliceStable(pr.Files, func(i, j int) bool {
		ci, cj := pr.Files[i].LinesAdded+pr.Files[i].LinesRemoved, pr.Files[j].LinesAdded+pr.Files[j].LinesRemoved
		if ci != cj {
			return ci > cj
		}
		return pr.Files[i].Edits > pr.Files[j].Edits
	})
	if len(pr.Files) > 15 {
		pr.Files = pr.Files[:15]
	}

	for _, t := range tools {
		pr.Tools = append(pr.Tools, *t)
	}
	sort.SliceStable(pr.Tools, func(i, j int) bool { return pr.Tools[i].Calls > pr.Tools[j].Calls })
	if len(pr.Tools) > 12 {
		pr.Tools = pr.Tools[:12]
	}

	return pr
}

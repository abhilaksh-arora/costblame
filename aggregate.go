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

	for _, pd := range projects {
		pr := aggregateProject(pd, pt, unpriced, timeline)
		if len(pr.Branches) == 0 {
			continue // no assistant turns
		}
		rep.Projects = append(rep.Projects, pr)
		rep.Totals.addTotals(pr.Totals)
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
	branches := map[string]*bacc{}
	providers := map[string]*pacc{}
	daily := map[string]*DayBucket{}
	projSessions := map[string]bool{}
	pr := ProjectReport{Folder: pd.Folder, Project: pd.RepoPath}

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

	pr.Sessions = len(projSessions)
	for _, b := range branches {
		b.Sessions = len(b.sessions)
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
	return pr
}

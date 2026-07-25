package main

import (
	"fmt"
	"io"
	"sort"
	"text/tabwriter"
	"time"
)

// renderBranchTable prints one project's per-branch spend (the default
// single-repo view).
func renderBranchTable(w io.Writer, pr ProjectReport) {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "BRANCH\tINPUT\tOUTPUT\tCACHE WR\tCACHE RD\tCOST\tSESSIONS\tDURATION")
	for _, b := range pr.Branches {
		fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%d\t$%.2f\t%d\t%s\n",
			b.Branch, b.Totals.Input, b.Totals.Output, b.Totals.CacheCreate,
			b.Totals.CacheRead, b.Totals.Cost, b.Sessions, humanDur(b.First, b.Last))
	}
	t := pr.Totals
	fmt.Fprintf(tw, "TOTAL\t%d\t%d\t%d\t%d\t$%.2f\t%d\t%s\n",
		t.Input, t.Output, t.CacheCreate, t.CacheRead, t.Cost, pr.Sessions, humanDur(pr.First, pr.Last))
	tw.Flush()
}

// renderActivity prints code-impact and workflow signals derived from
// Claude's edit tool calls — cost/edit, cost/100 changed lines, cache
// efficiency, rework loops, corrections, and tool error rate — plus the most
// heavily touched files. Claude-only, so it's skipped entirely when there's
// no edit activity to show (e.g. a Codex/Gemini-only project).
func renderActivity(w io.Writer, pr ProjectReport) {
	a := pr.Activity
	if a.Edits == 0 && a.ToolCalls == 0 {
		return
	}
	fmt.Fprintln(w, "\nACTIVITY (Claude only)")
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "EDITS\t+LINES\t-LINES\tCOST/EDIT\tCOST/100 LINES\tREWORK\tCORRECTIONS\tTOOL ERR%")
	errRate := "-"
	if a.ToolCalls > 0 {
		errRate = fmt.Sprintf("%.1f%%", a.ToolErrorRate*100)
	}
	costPerEdit, costPer100 := "-", "-"
	if a.Edits > 0 {
		costPerEdit = fmt.Sprintf("$%.3f", a.CostPerEdit)
	}
	if a.ChangedLines > 0 {
		costPer100 = fmt.Sprintf("$%.3f", a.CostPer100Lines)
	}
	fmt.Fprintf(tw, "%d\t%d\t%d\t%s\t%s\t%d\t%d\t%s\n",
		a.Edits, a.LinesAdded, a.LinesRemoved, costPerEdit, costPer100, a.ReworkLoops, a.Corrections, errRate)
	tw.Flush()

	if len(pr.Files) > 0 {
		fmt.Fprintln(w, "\nTOP FILES")
		ftw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
		fmt.Fprintln(ftw, "FILE\tEDITS\t+LINES\t-LINES\tSESSIONS")
		for _, f := range pr.Files {
			fmt.Fprintf(ftw, "%s\t%d\t%d\t%d\t%d\n", f.Path, f.Edits, f.LinesAdded, f.LinesRemoved, f.Sessions)
		}
		ftw.Flush()
	}

	if len(pr.Tools) > 0 {
		fmt.Fprintln(w, "\nTOOLS")
		ttw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
		fmt.Fprintln(ttw, "TOOL\tCALLS\tERRORS")
		for _, t := range pr.Tools {
			fmt.Fprintf(ttw, "%s\t%d\t%d\n", t.Name, t.Calls, t.Errors)
		}
		ttw.Flush()
	}
}

// renderProviderTable prints spend split by AI provider (claude/codex/gemini).
func renderProviderTable(w io.Writer, rep Report) {
	if len(rep.Providers) == 0 {
		return
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "PROVIDER\tINPUT\tOUTPUT\tCACHE RD\tCOST\tSHARE\tSESSIONS")
	total := rep.Totals.Cost
	for _, p := range rep.Providers {
		share := 0.0
		if total > 0 {
			share = p.Totals.Cost / total * 100
		}
		fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t$%.2f\t%.0f%%\t%d\n",
			p.Provider, p.Totals.Input, p.Totals.Output, p.Totals.CacheRead,
			p.Totals.Cost, share, p.Sessions)
	}
	fmt.Fprintf(tw, "TOTAL\t\t\t\t$%.2f\t\t%d\n", total, rep.Sessions)
	tw.Flush()
}

// renderProjectTable prints per-project spend (the --all view).
func renderProjectTable(w io.Writer, rep Report) {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "PROJECT\tINPUT\tOUTPUT\tCACHE WR\tCACHE RD\tCOST\tSESSIONS\tDURATION\tEDITS\tCOST/EDIT")
	for _, p := range rep.Projects {
		costPerEdit := "-"
		if p.Activity.Edits > 0 {
			costPerEdit = fmt.Sprintf("$%.3f", p.Activity.CostPerEdit)
		}
		fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%d\t$%.2f\t%d\t%s\t%d\t%s\n",
			p.Project, p.Totals.Input, p.Totals.Output, p.Totals.CacheCreate,
			p.Totals.CacheRead, p.Totals.Cost, p.Sessions, humanDur(p.First, p.Last),
			p.Activity.Edits, costPerEdit)
	}
	t := rep.Totals
	costPerEdit := "-"
	if rep.Activity.Edits > 0 {
		costPerEdit = fmt.Sprintf("$%.3f", rep.Activity.CostPerEdit)
	}
	fmt.Fprintf(tw, "TOTAL\t%d\t%d\t%d\t%d\t$%.2f\t%d\t\t%d\t%s\n",
		t.Input, t.Output, t.CacheCreate, t.CacheRead, t.Cost, rep.Sessions, rep.Activity.Edits, costPerEdit)
	tw.Flush()
}

// renderTimeTable prints spend bucketed by day or week across all projects.
func renderTimeTable(w io.Writer, rep Report, by string) {
	merged := map[string]*Totals{}
	var order []string
	for _, p := range rep.Projects {
		for _, d := range p.Daily {
			key := d.Date
			if by == "week" {
				key = weekKey(d.Date)
			}
			t := merged[key]
			if t == nil {
				t = &Totals{}
				merged[key] = t
				order = append(order, key)
			}
			t.addTotals(d.Totals)
		}
	}
	sort.Strings(order)

	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	label := "DAY"
	if by == "week" {
		label = "WEEK"
	}
	fmt.Fprintf(tw, "%s\tINPUT\tOUTPUT\tCACHE WR\tCACHE RD\tCOST\n", label)
	var tot Totals
	for _, k := range order {
		t := merged[k]
		fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%d\t$%.2f\n",
			k, t.Input, t.Output, t.CacheCreate, t.CacheRead, t.Cost)
		tot.addTotals(*t)
	}
	fmt.Fprintf(tw, "TOTAL\t%d\t%d\t%d\t%d\t$%.2f\n",
		tot.Input, tot.Output, tot.CacheCreate, tot.CacheRead, tot.Cost)
	tw.Flush()
}

// weekKey maps a YYYY-MM-DD date to an ISO year-week label (e.g. 2026-W27).
func weekKey(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	y, wk := t.ISOWeek()
	return fmt.Sprintf("%04d-W%02d", y, wk)
}

// humanDur renders the wall-clock span between first and last event.
func humanDur(first, last time.Time) string {
	if first.IsZero() || last.IsZero() {
		return "-"
	}
	d := last.Sub(first)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dd%dh", int(d.Hours())/24, int(d.Hours())%24)
	}
}

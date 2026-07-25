package main

import (
	"os"
	"path/filepath"
	"strings"
)

// ProjectData is the raw events for one project (repo path), before
// aggregation. Events may come from any provider (claude / codex / gemini);
// Edits/Corrections/Calls are Claude-only (Codex and Gemini logs don't carry
// the same tool payloads).
type ProjectData struct {
	Folder      string
	RepoPath    string
	Events      []Event
	Edits       []EditOp
	Corrections []CorrectionEvent
	Calls       []ToolCall
}

// activity bundles every provider's parsed output before it's grouped by
// repo — the merge point gatherAllActivity produces and groupByRepo consumes.
type activity struct {
	Events      []Event
	Edits       []EditOp
	Corrections []CorrectionEvent
	Calls       []ToolCall
}

// gatherAllActivity returns every billed event (all providers) plus Claude's
// edit/correction/tool-call activity. Each provider is best-effort: an
// uninstalled one contributes nothing rather than failing the whole scan.
func gatherAllActivity() (activity, error) {
	var act activity

	events, edits, corrections, calls, err := gatherClaudeActivity()
	if err != nil {
		return activity{}, err
	}
	act.Events = append(act.Events, events...)
	act.Edits = edits
	act.Corrections = corrections
	act.Calls = calls

	codex, err := CollectCodex()
	if err != nil {
		return activity{}, err
	}
	act.Events = append(act.Events, codex...)

	gemini, err := CollectGemini()
	if err != nil {
		return activity{}, err
	}
	act.Events = append(act.Events, gemini...)

	return act, nil
}

// gatherClaudeActivity walks every ~/.claude/projects folder and parses both
// token usage and edit/correction/tool-call activity from it.
func gatherClaudeActivity() ([]Event, []EditOp, []CorrectionEvent, []ToolCall, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	root := filepath.Join(home, ".claude", "projects")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil, nil, nil
		}
		return nil, nil, nil, nil, err
	}
	var events []Event
	var edits []EditOp
	var corrections []CorrectionEvent
	var calls []ToolCall
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		files, ferr := SessionFiles(dir)
		if ferr != nil || len(files) == 0 {
			continue
		}
		folderEvents, perr := ParseSessions(files)
		if perr != nil {
			continue
		}
		folderEdits, folderCorrections, folderCalls, aerr := ParseActivity(files)
		if aerr != nil {
			folderEdits, folderCorrections, folderCalls = nil, nil, nil
		}

		// Backfill cwd for anything missing it from a sibling in the same
		// folder, so grouping by repo path stays accurate.
		fallback := folderFallbackPath(folderEvents, e.Name())
		for i := range folderEvents {
			if folderEvents[i].Cwd == "" {
				folderEvents[i].Cwd = fallback
			}
		}
		for i := range folderEdits {
			if folderEdits[i].Cwd == "" {
				folderEdits[i].Cwd = fallback
			}
		}
		for i := range folderCorrections {
			if folderCorrections[i].Cwd == "" {
				folderCorrections[i].Cwd = fallback
			}
		}
		for i := range folderCalls {
			if folderCalls[i].Cwd == "" {
				folderCalls[i].Cwd = fallback
			}
		}

		events = append(events, folderEvents...)
		edits = append(edits, folderEdits...)
		corrections = append(corrections, folderCorrections...)
		calls = append(calls, folderCalls...)
	}
	return events, edits, corrections, calls, nil
}

// folderFallbackPath picks the repo path for a Claude project folder: the first
// cwd seen (reliable), else the lossy "/"->"-" decode of the folder name.
func folderFallbackPath(events []Event, folder string) string {
	for _, e := range events {
		if e.Cwd != "" {
			return e.Cwd
		}
	}
	return strings.ReplaceAll(folder, "-", "/")
}

// repoKey is the grouping key for a cwd: itself, or a sentinel when empty.
func repoKey(cwd string) string {
	if cwd != "" {
		return cwd
	}
	return "(unknown)"
}

// CollectAll gathers events from every provider and groups them by repo path.
func CollectAll() ([]ProjectData, error) {
	act, err := gatherAllActivity()
	if err != nil {
		return nil, err
	}
	projects := groupByRepo(act)

	// Ignoring is best-effort: a missing/unreadable ignore list just means
	// nothing is filtered, not a hard failure of the whole scan.
	ignored, _ := LoadIgnored()
	if len(ignored) == 0 {
		return projects, nil
	}
	kept := projects[:0]
	for _, p := range projects {
		if !ignored[p.RepoPath] {
			kept = append(kept, p)
		}
	}
	return kept, nil
}

// CollectRepo gathers events for a single repo (the --repo path) across all
// providers, matching on the recorded cwd.
func CollectRepo(repo string) (ProjectData, error) {
	abs, err := filepath.Abs(repo)
	if err != nil {
		return ProjectData{}, err
	}
	act, err := gatherAllActivity()
	if err != nil {
		return ProjectData{}, err
	}
	pd := ProjectData{Folder: filepath.Base(abs), RepoPath: abs}
	for _, e := range act.Events {
		if repoKey(e.Cwd) == abs {
			pd.Events = append(pd.Events, e)
		}
	}
	for _, e := range act.Edits {
		if repoKey(e.Cwd) == abs {
			pd.Edits = append(pd.Edits, e)
		}
	}
	for _, c := range act.Corrections {
		if repoKey(c.Cwd) == abs {
			pd.Corrections = append(pd.Corrections, c)
		}
	}
	for _, c := range act.Calls {
		if repoKey(c.Cwd) == abs {
			pd.Calls = append(pd.Calls, c)
		}
	}
	if len(pd.Events) == 0 {
		// Preserve the "no such folder" behaviour main.go relies on: if this
		// repo has no Claude session folder at all, report it as missing.
		if dir, derr := ProjectDir(repo); derr == nil {
			if _, serr := os.Stat(dir); serr != nil {
				return ProjectData{}, serr
			}
		}
	}
	return pd, nil
}

func groupByRepo(act activity) []ProjectData {
	idx := map[string]*ProjectData{}
	var order []string
	get := func(k string) *ProjectData {
		pd := idx[k]
		if pd == nil {
			pd = &ProjectData{Folder: filepath.Base(k), RepoPath: k}
			idx[k] = pd
			order = append(order, k)
		}
		return pd
	}
	for _, e := range act.Events {
		pd := get(repoKey(e.Cwd))
		pd.Events = append(pd.Events, e)
	}
	for _, e := range act.Edits {
		pd := get(repoKey(e.Cwd))
		pd.Edits = append(pd.Edits, e)
	}
	for _, c := range act.Corrections {
		pd := get(repoKey(c.Cwd))
		pd.Corrections = append(pd.Corrections, c)
	}
	for _, c := range act.Calls {
		pd := get(repoKey(c.Cwd))
		pd.Calls = append(pd.Calls, c)
	}
	out := make([]ProjectData, 0, len(order))
	for _, k := range order {
		out = append(out, *idx[k])
	}
	return out
}

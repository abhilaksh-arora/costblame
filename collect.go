package main

import (
	"os"
	"path/filepath"
	"strings"
)

// ProjectData is the raw events for one project (repo path), before
// aggregation. Events may come from any provider (claude / codex / gemini).
type ProjectData struct {
	Folder   string
	RepoPath string
	Events   []Event
}

// gatherAllEvents returns every billed event across all installed providers.
// Each provider is best-effort: an uninstalled one contributes nothing rather
// than failing the whole scan.
func gatherAllEvents() ([]Event, error) {
	var all []Event

	claude, err := gatherClaudeEvents()
	if err != nil {
		return nil, err
	}
	all = append(all, claude...)

	codex, err := CollectCodex()
	if err != nil {
		return nil, err
	}
	all = append(all, codex...)

	gemini, err := CollectGemini()
	if err != nil {
		return nil, err
	}
	all = append(all, gemini...)

	return all, nil
}

// gatherClaudeEvents walks every ~/.claude/projects folder and parses it.
func gatherClaudeEvents() ([]Event, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	root := filepath.Join(home, ".claude", "projects")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Event
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		files, ferr := SessionFiles(dir)
		if ferr != nil || len(files) == 0 {
			continue
		}
		events, perr := ParseSessions(files)
		if perr != nil {
			continue
		}
		// Backfill cwd for any event missing it from a sibling in the same
		// folder, so grouping by repo path stays accurate.
		fallback := folderFallbackPath(events, e.Name())
		for i := range events {
			if events[i].Cwd == "" {
				events[i].Cwd = fallback
			}
		}
		out = append(out, events...)
	}
	return out, nil
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

// repoKey is the grouping key for an event: its recorded cwd, or a sentinel.
func repoKey(e Event) string {
	if e.Cwd != "" {
		return e.Cwd
	}
	return "(unknown)"
}

// CollectAll gathers events from every provider and groups them by repo path.
func CollectAll() ([]ProjectData, error) {
	events, err := gatherAllEvents()
	if err != nil {
		return nil, err
	}
	return groupByRepo(events), nil
}

// CollectRepo gathers events for a single repo (the --repo path) across all
// providers, matching on the recorded cwd.
func CollectRepo(repo string) (ProjectData, error) {
	abs, err := filepath.Abs(repo)
	if err != nil {
		return ProjectData{}, err
	}
	events, err := gatherAllEvents()
	if err != nil {
		return ProjectData{}, err
	}
	var mine []Event
	for _, e := range events {
		if repoKey(e) == abs {
			mine = append(mine, e)
		}
	}
	if len(mine) == 0 {
		// Preserve the "no such folder" behaviour main.go relies on: if this
		// repo has no Claude session folder at all, report it as missing.
		if dir, derr := ProjectDir(repo); derr == nil {
			if _, serr := os.Stat(dir); serr != nil {
				return ProjectData{}, serr
			}
		}
	}
	return ProjectData{Folder: filepath.Base(abs), RepoPath: abs, Events: mine}, nil
}

func groupByRepo(events []Event) []ProjectData {
	idx := map[string]*ProjectData{}
	var order []string
	for _, e := range events {
		k := repoKey(e)
		pd := idx[k]
		if pd == nil {
			pd = &ProjectData{Folder: filepath.Base(k), RepoPath: k}
			idx[k] = pd
			order = append(order, k)
		}
		pd.Events = append(pd.Events, e)
	}
	out := make([]ProjectData, 0, len(order))
	for _, k := range order {
		out = append(out, *idx[k])
	}
	return out
}

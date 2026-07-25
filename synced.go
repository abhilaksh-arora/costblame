package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// syncedPath returns ~/.costblame/synced.json — an ordered, deduplicated list
// of repo paths that plain `costblame sync` (no --all, no --repo) and plain
// `costblame serve` (same) remember across runs. Running sync/serve bare in a
// repo adds it to this list; the report shown is always the union of
// everything in it, not just the repo you're standing in right now. This is
// deliberately separate from --repo (an explicit, one-off, non-persistent
// list) and --all (everything under ~/.claude/projects, no memory needed).
func syncedPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".costblame", "synced.json"), nil
}

// LoadSynced returns the synced list in the order repos were added. A
// missing file (nothing synced yet) is not an error.
func LoadSynced() ([]string, error) {
	path, err := syncedPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var list []string
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func saveSynced(list []string) error {
	path, err := syncedPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(list, "", "  ")
	return os.WriteFile(path, b, 0o644)
}

// AddSynced adds abs to the synced list if it isn't already there (a no-op
// write otherwise) and returns the resulting list.
func AddSynced(abs string) ([]string, error) {
	list, err := LoadSynced()
	if err != nil {
		return nil, err
	}
	for _, p := range list {
		if p == abs {
			return list, nil // already synced; don't rewrite the file every call
		}
	}
	list = append(list, abs)
	if err := saveSynced(list); err != nil {
		return nil, err
	}
	return list, nil
}

// RemoveSynced drops abs from the synced list.
func RemoveSynced(abs string) ([]string, bool, error) {
	list, err := LoadSynced()
	if err != nil {
		return nil, false, err
	}
	out := list[:0]
	found := false
	for _, p := range list {
		if p == abs {
			found = true
			continue
		}
		out = append(out, p)
	}
	if !found {
		return list, false, nil
	}
	if err := saveSynced(out); err != nil {
		return nil, false, err
	}
	return out, true, nil
}

// projectsForSynced collects ProjectData for every repo in list. A repo whose
// Claude session folder no longer exists (renamed/deleted since it was
// synced) is skipped rather than treated as fatal.
func projectsForSynced(list []string) ([]ProjectData, error) {
	var projects []ProjectData
	for _, r := range list {
		pd, err := CollectRepo(r)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		projects = append(projects, pd)
	}
	return projects, nil
}

func absCwd() (string, error) {
	return filepath.Abs(".")
}

// runSynced is `costblame synced` — lists the persistent set sync/serve show
// by default, in the order repos were added.
func runSynced(_ []string) {
	list, err := LoadSynced()
	if err != nil {
		fatal("reading synced list: %v", err)
	}
	if len(list) == 0 {
		fmt.Println("nothing synced yet — run `costblame sync` in a repo to add it")
		return
	}
	for _, p := range list {
		fmt.Println(p)
	}
}

// runForget is `costblame forget [DIR]` — removes a repo (default: the
// current directory) from the synced list. It doesn't touch any logs.
func runForget(args []string) {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		fatal("resolving %q: %v", dir, err)
	}
	_, found, err := RemoveSynced(abs)
	if err != nil {
		fatal("updating synced list: %v", err)
	}
	if !found {
		fmt.Printf("not in the synced list: %s\n", abs)
		return
	}
	fmt.Printf("forgot %s\n", abs)
}

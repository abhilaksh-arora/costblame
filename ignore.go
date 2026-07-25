package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ignorePath returns ~/.costblame/ignored.json — a plain array of absolute
// repo paths excluded from the "everything" views (--all / sync --all /
// serve --all). It never affects an explicit `--repo` (or bare, cwd) request:
// ignoring a repo only trims it out of the aggregate, it doesn't hide it.
func ignorePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".costblame", "ignored.json"), nil
}

// LoadIgnored returns the set of ignored absolute repo paths. A missing file
// (nothing ignored yet) is not an error.
func LoadIgnored() (map[string]bool, error) {
	path, err := ignorePath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	var list []string
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(list))
	for _, p := range list {
		out[p] = true
	}
	return out, nil
}

func saveIgnored(set map[string]bool) error {
	path, err := ignorePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	list := make([]string, 0, len(set))
	for p := range set {
		list = append(list, p)
	}
	b, _ := json.MarshalIndent(list, "", "  ")
	return os.WriteFile(path, b, 0o644)
}

// runIgnore is `costblame ignore [DIR]` — excludes a repo (default: the
// current directory) from --all / sync --all / serve --all.
func runIgnore(args []string) {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		fatal("resolving %q: %v", dir, err)
	}
	set, err := LoadIgnored()
	if err != nil {
		fatal("reading ignore list: %v", err)
	}
	if set[abs] {
		fmt.Printf("already ignored: %s\n", abs)
		return
	}
	set[abs] = true
	if err := saveIgnored(set); err != nil {
		fatal("saving ignore list: %v", err)
	}
	fmt.Printf("ignoring %s\n", abs)
	fmt.Println("dropped from --all / sync --all / serve --all; costblame sync --repo " + abs + " still shows it directly.")
}

// runUnignore is `costblame unignore [DIR]` — reverses runIgnore.
func runUnignore(args []string) {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		fatal("resolving %q: %v", dir, err)
	}
	set, err := LoadIgnored()
	if err != nil {
		fatal("reading ignore list: %v", err)
	}
	if !set[abs] {
		fmt.Printf("not ignored: %s\n", abs)
		return
	}
	delete(set, abs)
	if err := saveIgnored(set); err != nil {
		fatal("saving ignore list: %v", err)
	}
	fmt.Printf("no longer ignoring %s\n", abs)
}

// runIgnored is `costblame ignored` — lists everything currently excluded.
func runIgnored(_ []string) {
	set, err := LoadIgnored()
	if err != nil {
		fatal("reading ignore list: %v", err)
	}
	if len(set) == 0 {
		fmt.Println("nothing is ignored")
		return
	}
	for p := range set {
		fmt.Println(p)
	}
}

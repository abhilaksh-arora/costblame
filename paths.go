package main

import (
	"os"
	"path/filepath"
	"strings"
)

// ProjectDir maps an absolute repo path to its ~/.claude/projects folder.
// Claude Code encodes the folder name by replacing every "/" in the absolute
// path with "-" (leading slash becomes a leading "-").
func ProjectDir(repo string) (string, error) {
	abs, err := filepath.Abs(repo)
	if err != nil {
		return "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	encoded := strings.ReplaceAll(abs, "/", "-")
	return filepath.Join(home, ".claude", "projects", encoded), nil
}

// SessionFiles lists the .jsonl session files in a project dir.
func SessionFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	return files, nil
}

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Gemini CLI writes chat logs under ~/.gemini/tmp/<projectHash>/chats/*.jsonl
// (sometimes one level deeper in a per-session subdir). Each assistant turn is
// one line with type=="gemini" carrying a tokens{} block and the model. As with
// Codex there is no gitBranch — attribution is per project.
//
// ~/.gemini/projects.json maps "<abs repo path>" -> "<projectHash>", so we
// invert it to recover the real path from the folder name.
//
// Token mapping: Gemini's tokens.input INCLUDES cached; tokens.output does NOT
// include thoughts (reasoning). Billable output = output + thoughts; cached maps
// to CacheRead; non-cached input = input - cached. No separate cache-write
// charge.

type geminiLine struct {
	ID        string    `json:"id"`
	SessionID string    `json:"sessionId"` // present on the per-file metadata line
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Model     string    `json:"model"`
	Tokens    *struct {
		Input    int `json:"input"`
		Output   int `json:"output"`
		Cached   int `json:"cached"`
		Thoughts int `json:"thoughts"`
		Tool     int `json:"tool"`
		Total    int `json:"total"`
	} `json:"tokens"`
}

// CollectGemini returns Events for every Gemini chat turn, Provider "gemini".
func CollectGemini() ([]Event, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	root := filepath.Join(home, ".gemini", "tmp")
	if _, err := os.Stat(root); err != nil {
		return nil, nil // Gemini not installed — not an error
	}
	hashToPath := geminiHashToPath(filepath.Join(home, ".gemini", "projects.json"))

	seen := make(map[string]bool)
	var events []Event
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		// The project folder is the first path segment under tmp/.
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil
		}
		seg := strings.SplitN(rel, string(filepath.Separator), 2)[0]
		if seg == "background-processes" {
			return nil // not a chat log
		}
		repo := hashToPath[seg]
		if repo == "" {
			repo = seg // fall back to the folder name
		}
		if perr := parseGeminiFile(path, repo, seen, &events); perr != nil {
			fmt.Fprintf(os.Stderr, "warning: %s skipped (%v)\n", path, perr)
		}
		return nil
	})
	return events, err
}

// geminiHashToPath inverts projects.json ({ "projects": { "<path>": "<hash>" } })
// into hash -> path.
func geminiHashToPath(path string) map[string]string {
	out := map[string]string{}
	b, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var pj struct {
		Projects map[string]string `json:"projects"`
	}
	if json.Unmarshal(b, &pj) != nil {
		return out
	}
	for repoPath, hash := range pj.Projects {
		out[hash] = repoPath
	}
	return out
}

func parseGeminiFile(path, repo string, seen map[string]bool, out *[]Event) error {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()

	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	// A chat file opens with a metadata line carrying the real sessionId; use
	// it so sessions are counted per chat, not per project folder. Fall back to
	// the file name if absent.
	session := strings.TrimSuffix(filepath.Base(path), ".jsonl")

	for sc.Scan() {
		b := sc.Bytes()
		if len(b) == 0 {
			continue
		}
		var l geminiLine
		if err := json.Unmarshal(b, &l); err != nil {
			continue
		}
		if l.SessionID != "" {
			session = l.SessionID
		}
		if l.Type != "gemini" || l.Tokens == nil || l.ID == "" {
			continue
		}
		if seen[l.ID] {
			continue // same turn can appear in a session file and its snapshot dir
		}
		seen[l.ID] = true

		nonCachedInput := l.Tokens.Input - l.Tokens.Cached
		if nonCachedInput < 0 {
			nonCachedInput = 0
		}
		*out = append(*out, Event{
			Provider:  "gemini",
			MessageID: l.ID,
			SessionID: session,
			Branch:    "(n/a)",
			Cwd:       repo,
			Model:     l.Model,
			Timestamp: l.Timestamp,
			Input:     nonCachedInput,
			Output:    l.Tokens.Output + l.Tokens.Thoughts, // thoughts billed as output
			CacheRead: l.Tokens.Cached,
		})
	}
	return sc.Err()
}

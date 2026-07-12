package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Event is one billed assistant turn, deduplicated by message id. A single
// assistant turn is written to the .jsonl as several lines (one per content
// block: thinking / text / tool_use), and every line repeats the identical
// usage — so counting per line overcounts. We key on message.id and keep the
// first occurrence.
type Event struct {
	Provider      string // "claude" | "codex" | "gemini"
	MessageID     string
	SessionID     string
	Branch        string
	Cwd           string // working dir recorded on the message; recovers the repo path
	Model         string
	Timestamp     time.Time
	Input         int
	Output        int
	CacheCreate   int // total cache-write tokens (5m + 1h)
	CacheCreate1h int // subset written with the 1-hour TTL (priced higher)
	CacheRead     int
}

// rawLine is the subset of a .jsonl record we read.
type rawLine struct {
	Type      string    `json:"type"`
	SessionID string    `json:"sessionId"`
	GitBranch string    `json:"gitBranch"`
	Cwd       string    `json:"cwd"`
	Timestamp time.Time `json:"timestamp"`
	Message   struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage struct {
			Input       int `json:"input_tokens"`
			Output      int `json:"output_tokens"`
			CacheCreate int `json:"cache_creation_input_tokens"`
			CacheRead   int `json:"cache_read_input_tokens"`
			// Breakdown of cache_creation by TTL. Present on recent logs; older
			// logs may omit it, in which case the whole total is treated as 5m.
			CacheCreation struct {
				Ephemeral5m int `json:"ephemeral_5m_input_tokens"`
				Ephemeral1h int `json:"ephemeral_1h_input_tokens"`
			} `json:"cache_creation"`
		} `json:"usage"`
	} `json:"message"`
}

// ParseSessions reads every file and returns deduplicated assistant events.
// Malformed lines are skipped with a warning to stderr rather than aborting.
func ParseSessions(files []string) ([]Event, error) {
	seen := make(map[string]bool)
	var events []Event
	for _, f := range files {
		if err := parseFile(f, seen, &events); err != nil {
			return nil, err
		}
	}
	return events, nil
}

func parseFile(path string, seen map[string]bool, out *[]Event) error {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()

	sc := bufio.NewScanner(fh)
	// Lines can carry large tool results; raise the token limit well above the
	// 64KB default.
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	lineNo := 0
	for sc.Scan() {
		lineNo++
		b := sc.Bytes()
		if len(b) == 0 {
			continue
		}
		var r rawLine
		if err := json.Unmarshal(b, &r); err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s:%d skipped (bad json: %v)\n", path, lineNo, err)
			continue
		}
		if r.Type != "assistant" || r.Message.ID == "" {
			continue
		}
		if seen[r.Message.ID] {
			continue // repeated content block of an already-counted turn
		}
		seen[r.Message.ID] = true

		branch := r.GitBranch
		if branch == "" {
			branch = "(unknown)"
		}
		// The 1h portion can't exceed the reported total; clamp defensively so
		// a malformed breakdown never over-prices.
		h1 := r.Message.Usage.CacheCreation.Ephemeral1h
		if h1 > r.Message.Usage.CacheCreate {
			h1 = r.Message.Usage.CacheCreate
		}
		*out = append(*out, Event{
			Provider:      "claude",
			MessageID:     r.Message.ID,
			SessionID:     r.SessionID,
			Branch:        branch,
			Cwd:           r.Cwd,
			Model:         r.Message.Model,
			Timestamp:     r.Timestamp,
			Input:         r.Message.Usage.Input,
			Output:        r.Message.Usage.Output,
			CacheCreate:   r.Message.Usage.CacheCreate,
			CacheCreate1h: h1,
			CacheRead:     r.Message.Usage.CacheRead,
		})
	}
	return sc.Err()
}

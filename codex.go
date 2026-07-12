package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Codex (OpenAI) writes one JSONL "rollout" file per session under
// ~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl. Unlike Claude's logs there is
// no gitBranch — only a cwd — so Codex spend is attributed per project, not per
// branch.
//
// Relevant record types within a rollout file:
//   - session_meta : payload.{id, cwd, timestamp}
//   - turn_context : payload.model                (repeats; last one wins)
//   - event_msg    : payload.type == "token_count", payload.info holds the
//                    running usage. info.total_token_usage is cumulative for the
//                    whole session, so the final populated one is the session
//                    total — no per-line dedup needed.
//
// OpenAI convention: input_tokens INCLUDES cached_input_tokens, and
// output_tokens already INCLUDES reasoning_output_tokens. So billable
// non-cached input = input - cached, and cached maps to our CacheRead. There is
// no separate cache-write charge (prompt caching is automatic).

type codexLine struct {
	Timestamp time.Time       `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type codexMeta struct {
	ID        string    `json:"id"`
	Cwd       string    `json:"cwd"`
	Timestamp time.Time `json:"timestamp"`
}

type codexTurnCtx struct {
	Model string `json:"model"`
}

type codexTokenCount struct {
	Type string `json:"type"`
	Info *struct {
		Total struct {
			Input     int `json:"input_tokens"`
			Cached    int `json:"cached_input_tokens"`
			Output    int `json:"output_tokens"`
			Reasoning int `json:"reasoning_output_tokens"`
			Total     int `json:"total_tokens"`
		} `json:"total_token_usage"`
	} `json:"info"`
}

// CollectCodex returns one Event per Codex session (keyed by session id), with
// Provider "codex" and Cwd set for per-project attribution.
func CollectCodex() ([]Event, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	root := filepath.Join(home, ".codex", "sessions")
	if _, err := os.Stat(root); err != nil {
		return nil, nil // Codex not installed / no sessions — not an error
	}

	var events []Event
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		ev, ok, perr := parseCodexFile(path)
		if perr != nil {
			fmt.Fprintf(os.Stderr, "warning: %s skipped (%v)\n", path, perr)
			return nil
		}
		if ok {
			events = append(events, ev)
		}
		return nil
	})
	return events, err
}

func parseCodexFile(path string) (Event, bool, error) {
	fh, err := os.Open(path)
	if err != nil {
		return Event{}, false, err
	}
	defer fh.Close()

	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	var (
		meta  codexMeta
		model string
		total codexTokenCount // last populated token_count wins
		haveUsage bool
	)
	for sc.Scan() {
		b := sc.Bytes()
		if len(b) == 0 {
			continue
		}
		var l codexLine
		if err := json.Unmarshal(b, &l); err != nil {
			continue // tolerate a stray bad line
		}
		switch l.Type {
		case "session_meta":
			var m codexMeta
			if json.Unmarshal(l.Payload, &m) == nil {
				meta = m
			}
		case "turn_context":
			var c codexTurnCtx
			if json.Unmarshal(l.Payload, &c) == nil && c.Model != "" {
				model = c.Model
			}
		case "event_msg":
			var tc codexTokenCount
			if json.Unmarshal(l.Payload, &tc) == nil && tc.Type == "token_count" && tc.Info != nil {
				total = tc
				haveUsage = true
			}
		}
	}
	if err := sc.Err(); err != nil {
		return Event{}, false, err
	}
	if !haveUsage || total.Info.Total.Total == 0 {
		return Event{}, false, nil // session with no billable usage
	}

	u := total.Info.Total
	ts := meta.Timestamp
	if ts.IsZero() {
		ts = time.Time{}
	}
	nonCachedInput := u.Input - u.Cached
	if nonCachedInput < 0 {
		nonCachedInput = 0
	}
	return Event{
		Provider:  "codex",
		SessionID: meta.ID,
		MessageID: meta.ID, // session-level; unique per file
		Branch:    "(n/a)", // Codex logs carry no branch
		Cwd:       meta.Cwd,
		Model:     model,
		Timestamp: ts,
		Input:     nonCachedInput,
		Output:    u.Output, // already includes reasoning tokens
		CacheRead: u.Cached,
		// No separate cache-write charge in OpenAI's model.
	}, true, nil
}

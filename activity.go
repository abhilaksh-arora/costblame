package main

import (
	"bufio"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"time"
)

// EditOp is one file-editing tool call (Edit/MultiEdit/Write/NotebookEdit)
// parsed from a Claude session log, with a crude line-count diff.
type EditOp struct {
	SessionID string
	Branch    string
	Cwd       string
	FilePath  string
	Added     int
	Removed   int
	Timestamp time.Time
}

// CorrectionEvent marks a user turn that reads as a correction to the
// assistant's prior work — never the session's first message, since that
// can't be correcting anything yet.
type CorrectionEvent struct {
	SessionID string
	Branch    string
	Cwd       string
	Timestamp time.Time
}

// ToolCall is one tool_use invocation plus whether its tool_result (on a
// later line) came back as an error.
type ToolCall struct {
	Name      string
	SessionID string
	Branch    string
	Cwd       string
	Timestamp time.Time
	IsError   bool
}

// correctionRE mirrors the heuristic used by comparable tools (e.g. a
// leading "no"/"wrong"/"undo"/"actually" etc.) for detecting a user turn
// that's pushing back on the assistant's prior work rather than starting a
// new, unrelated request.
var correctionRE = regexp.MustCompile(`(?i)(?:^|\b)(?:no[,:]?|nope|wrong|incorrect|not what i|that(?:'s| is) not|you (?:missed|ignored|changed)|actually[,:]?|instead[,:]?|stop[,:]?|undo|revert|go back|don(?:'t| not)|i said|please fix that)(?:\b|$)`)

// activityLine is the subset of a .jsonl record activity parsing needs.
type activityLine struct {
	Type      string    `json:"type"`
	SessionID string    `json:"sessionId"`
	GitBranch string    `json:"gitBranch"`
	Cwd       string    `json:"cwd"`
	Timestamp time.Time `json:"timestamp"`
	Message   struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// contentBlock covers the union of shapes seen in an assistant or user
// message's content array: tool_use, tool_result, and text.
type contentBlock struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`          // tool_use
	Name      string          `json:"name"`        // tool_use
	Input     json.RawMessage `json:"input"`       // tool_use
	Text      string          `json:"text"`        // text
	ToolUseID string          `json:"tool_use_id"` // tool_result
	IsError   bool            `json:"is_error"`    // tool_result
}

// editToolInput covers Edit, and the per-edit shape reused by MultiEdit.
type editToolInput struct {
	FilePath     string `json:"file_path"`
	OldString    string `json:"old_string"`
	NewString    string `json:"new_string"`
	Content      string `json:"content"`    // Write
	NewSource    string `json:"new_source"` // NotebookEdit
	NotebookPath string `json:"notebook_path"`
	EditMode     string `json:"edit_mode"` // NotebookEdit: replace|insert|delete
	Edits        []struct {
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	} `json:"edits"` // MultiEdit
}

// lineCount returns the line count of a non-empty string (0 for empty).
func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// editOpsFor turns one Edit/MultiEdit/Write/NotebookEdit tool_use block into
// zero or more EditOps. Diffing is line-count-based, not a real LCS diff —
// good enough for a changed-lines / cost-per-edit metric, not for rendering
// a patch.
func editOpsFor(name string, input json.RawMessage) []struct {
	path           string
	added, removed int
} {
	var in editToolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil
	}
	type r = struct {
		path           string
		added, removed int
	}
	switch name {
	case "Edit":
		return []r{{in.FilePath, lineCount(in.NewString), lineCount(in.OldString)}}
	case "MultiEdit":
		out := make([]r, 0, len(in.Edits))
		for _, e := range in.Edits {
			out = append(out, r{in.FilePath, lineCount(e.NewString), lineCount(e.OldString)})
		}
		return out
	case "Write":
		// The old file content isn't in the payload, so — like every other
		// tool that lacks it — everything visible is counted as an addition.
		return []r{{in.FilePath, lineCount(in.Content), 0}}
	case "NotebookEdit":
		if in.EditMode == "delete" {
			return []r{{in.NotebookPath, 0, 1}}
		}
		return []r{{in.NotebookPath, lineCount(in.NewSource), 0}}
	}
	return nil
}

// ParseActivity re-scans the same Claude session files ParseSessions reads,
// extracting file-editing tool calls, user corrections, and tool call/error
// counts. Kept as a separate pass (rather than folded into sessions.go) so
// token pricing and activity tracking stay independently testable.
func ParseActivity(files []string) (edits []EditOp, corrections []CorrectionEvent, calls []ToolCall, err error) {
	for _, f := range files {
		if perr := parseActivityFile(f, &edits, &corrections, &calls); perr != nil {
			return nil, nil, nil, perr
		}
	}
	return edits, corrections, calls, nil
}

func parseActivityFile(path string, edits *[]EditOp, corrections *[]CorrectionEvent, calls *[]ToolCall) error {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()

	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	// localCalls holds *ToolCall (not ToolCall) so that pending's pointers stay
	// valid regardless of how this slice itself grows/reallocates — only the
	// slice-of-pointers backing array moves, never the pointed-to structs.
	var localCalls []*ToolCall
	pending := map[string]*ToolCall{} // tool_use id -> call, until its tool_result (or EOF) resolves it
	seenFirstUser := map[string]bool{}

	for sc.Scan() {
		b := sc.Bytes()
		if len(b) == 0 {
			continue
		}
		var l activityLine
		if err := json.Unmarshal(b, &l); err != nil {
			continue // sessions.go already warns for malformed lines
		}
		branch := l.GitBranch
		if branch == "" {
			branch = "(unknown)"
		}

		switch {
		case l.Type == "assistant":
			var blocks []contentBlock
			if json.Unmarshal(l.Message.Content, &blocks) != nil {
				continue
			}
			for _, blk := range blocks {
				if blk.Type != "tool_use" {
					continue
				}
				call := &ToolCall{Name: blk.Name, SessionID: l.SessionID, Branch: branch, Cwd: l.Cwd, Timestamp: l.Timestamp}
				localCalls = append(localCalls, call)
				if blk.ID != "" {
					pending[blk.ID] = call
				}
				for _, op := range editOpsFor(blk.Name, blk.Input) {
					if op.path == "" {
						continue
					}
					*edits = append(*edits, EditOp{
						SessionID: l.SessionID, Branch: branch, Cwd: l.Cwd,
						FilePath: op.path, Added: op.added, Removed: op.removed,
						Timestamp: l.Timestamp,
					})
				}
			}

		case l.Type == "user":
			var blocks []contentBlock
			var text string
			if json.Unmarshal(l.Message.Content, &blocks) == nil {
				var texts []string
				for _, blk := range blocks {
					switch blk.Type {
					case "tool_result":
						if c, ok := pending[blk.ToolUseID]; ok {
							c.IsError = blk.IsError
						}
					case "text":
						if blk.Text != "" {
							texts = append(texts, blk.Text)
						}
					}
				}
				text = strings.Join(texts, "\n")
			} else {
				// content was a plain JSON string, not a block array
				_ = json.Unmarshal(l.Message.Content, &text)
			}
			if text == "" {
				continue // pure tool_result carrier line — not a real user turn
			}
			if seenFirstUser[l.SessionID] && correctionRE.MatchString(text) {
				*corrections = append(*corrections, CorrectionEvent{
					SessionID: l.SessionID, Branch: branch, Cwd: l.Cwd, Timestamp: l.Timestamp,
				})
			}
			seenFirstUser[l.SessionID] = true
		}
	}
	for _, c := range localCalls {
		*calls = append(*calls, *c)
	}
	return sc.Err()
}

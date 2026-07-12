package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config is the user's stored plan selection, used to reframe API-equivalent
// cost against a flat subscription price.
type Config struct {
	Plan       string  `json:"plan"`        // key from planCatalog
	PlanName   string  `json:"plan_name"`   // display name
	PriceLabel string  `json:"price_label"` // e.g. "$20/mo"
	MonthlyUSD float64 `json:"monthly_usd"` // 0 for API/pay-as-you-go/skip
}

type planDef struct {
	key, name, priceLabel string
	usd                   float64
}

// planCatalog is the hardcoded plan → flat-price lookup. Update when Anthropic
// changes plan prices.
var planCatalog = []planDef{
	{"pro", "Claude Pro", "$20/mo", 20},
	{"max5", "Claude Max 5x", "$100/mo", 100},
	{"max20", "Claude Max 20x", "$200/mo", 200},
	{"team_std", "Claude Team (Standard)", "$25/mo per seat", 25},
	{"team_prem", "Claude Team (Premium)", "$125/mo per seat", 125},
	{"api", "API / pay-as-you-go", "", 0},
	{"skip", "(not set)", "", 0},
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".costblame", "config.json"), nil
}

// runUninstall removes the installed binary and the ~/.costblame config. It
// never touches the session logs (~/.claude, ~/.codex, ~/.gemini).
func runUninstall(_ []string) {
	if home, err := os.UserHomeDir(); err == nil {
		dir := filepath.Join(home, ".costblame")
		if _, statErr := os.Stat(dir); statErr == nil {
			if os.RemoveAll(dir) == nil {
				fmt.Printf("removed %s\n", dir)
			}
		}
	}

	exe, err := os.Executable()
	if err != nil {
		fatal("could not locate the binary to remove: %v", err)
	}
	if resolved, e := filepath.EvalSymlinks(exe); e == nil {
		exe = resolved // remove the real file, not a symlink to it
	}
	if err := os.Remove(exe); err != nil {
		fatal("could not remove %s: %v\n  remove it manually: rm %s", exe, err, exe)
	}
	fmt.Printf("removed %s\n", exe)
	fmt.Println("costblame uninstalled. Your Claude/Codex/Gemini logs are untouched.")
}

// LoadConfig returns the stored config, or (nil, nil) if none exists yet.
func LoadConfig() (*Config, error) {
	path, err := configPath()
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
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// SaveConfig writes the config to ~/.costblame/config.json.
func SaveConfig(c *Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(path, b, 0o644)
}

// stdinIsTTY reports whether stdin is an interactive terminal (so we can prompt
// safely without blocking a pipe).
func stdinIsTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// ensureConfig returns the stored config, prompting once (and saving) on first
// interactive use. In a non-interactive context with no config, returns nil so
// the caller falls back to raw API-equivalent framing.
func ensureConfig() *Config {
	if c, _ := LoadConfig(); c != nil {
		return c
	}
	if !stdinIsTTY() {
		return nil
	}
	fmt.Fprintln(os.Stderr, "First run — let's set your Claude plan so costs are framed against what you pay.")
	c := promptPlan()
	if c != nil {
		if err := SaveConfig(c); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not save config: %v\n", err)
		} else {
			path, _ := configPath()
			fmt.Fprintf(os.Stderr, "saved to %s (run `costblame configure` to change)\n\n", path)
		}
	}
	return c
}

// promptPlan renders the plan menu and reads a choice from stdin.
func promptPlan() *Config {
	menu := []string{
		"1] Claude Pro ($20/mo)",
		"2] Claude Max 5x ($100/mo)",
		"3] Claude Max 20x ($200/mo)",
		"4] Claude Team — Standard ($25/mo per seat)",
		"5] Claude Team — Premium ($125/mo per seat)",
		"6] API / pay-as-you-go",
		"7] Skip",
	}
	keys := []string{"pro", "max5", "max20", "team_std", "team_prem", "api", "skip"}

	r := bufio.NewReader(os.Stdin)
	for attempts := 0; attempts < 5; attempts++ {
		fmt.Fprintln(os.Stderr, "\nWhat's your Claude plan?")
		for _, line := range menu {
			fmt.Fprintln(os.Stderr, "  ["+line)
		}
		fmt.Fprint(os.Stderr, "Choice [1-7]: ")

		line, err := r.ReadString('\n')
		if err != nil && line == "" {
			return catalogConfig("skip")
		}
		choice := strings.TrimSpace(line)
		idx := -1
		switch choice {
		case "1":
			idx = 0
		case "2":
			idx = 1
		case "3":
			idx = 2
		case "4":
			idx = 3
		case "5":
			idx = 4
		case "6":
			idx = 5
		case "7", "":
			idx = 6
		}
		if idx >= 0 {
			return catalogConfig(keys[idx])
		}
		fmt.Fprintln(os.Stderr, "  (enter a number 1-7)")
	}
	return catalogConfig("skip")
}

func catalogConfig(key string) *Config {
	for _, p := range planCatalog {
		if p.key == key {
			return &Config{Plan: p.key, PlanName: p.name, PriceLabel: p.priceLabel, MonthlyUSD: p.usd}
		}
	}
	return &Config{Plan: "skip", PlanName: "(not set)"}
}

// runConfigure is the `costblame configure` subcommand.
func runConfigure(_ []string) {
	if !stdinIsTTY() {
		fatal("configure needs an interactive terminal")
	}
	c := promptPlan()
	if err := SaveConfig(c); err != nil {
		fatal("saving config: %v", err)
	}
	path, _ := configPath()
	fmt.Fprintf(os.Stderr, "\nSaved plan: %s → %s\n", c.PlanName, path)
}

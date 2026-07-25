package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ProviderPlan is one provider's stored flat-price subscription selection.
type ProviderPlan struct {
	Plan       string  `json:"plan"`        // key from planCatalogs[provider]
	PlanName   string  `json:"plan_name"`   // display name
	PriceLabel string  `json:"price_label"` // e.g. "$20/mo"
	MonthlyUSD float64 `json:"monthly_usd"` // 0 for API/pay-as-you-go/skip
}

// Config is the user's stored plan selection, one per provider (claude /
// codex / gemini), used to reframe each provider's API-equivalent cost
// against what its subscription actually costs.
type Config struct {
	Plans map[string]ProviderPlan `json:"plans"`
}

type planDef struct {
	key, name, priceLabel string
	usd                   float64
}

// planCatalogs is the hardcoded plan → flat-price lookup per provider. Update
// when a provider changes plan prices.
//
// Codex rates track OpenAI's ChatGPT subscription tiers (what actually backs
// Codex CLI usage for most people); Gemini tracks Google AI / Gemini Code
// Assist. Verified 2026-07-25 — check the provider's own pricing page if a
// number looks stale.
var planCatalogs = map[string][]planDef{
	"claude": {
		{"pro", "Claude Pro", "$20/mo", 20},
		{"max5", "Claude Max 5x", "$100/mo", 100},
		{"max20", "Claude Max 20x", "$200/mo", 200},
		{"team_std", "Claude Team (Standard)", "$25/mo per seat", 25},
		{"team_prem", "Claude Team (Premium)", "$125/mo per seat", 125},
		{"api", "API / pay-as-you-go", "", 0},
		{"skip", "(not set)", "", 0},
	},
	"codex": {
		{"plus", "ChatGPT Plus", "$20/mo", 20},
		{"pro", "ChatGPT Pro", "$200/mo", 200},
		{"team", "ChatGPT Team", "$25/mo per seat", 25},
		{"api", "API / pay-as-you-go", "", 0},
		{"skip", "(not set)", "", 0},
	},
	"gemini": {
		{"ai_pro", "Google AI Pro", "$19.99/mo", 19.99},
		{"ai_ultra", "Google AI Ultra", "$249.99/mo", 249.99},
		{"code_assist_std", "Gemini Code Assist Standard", "$19/mo per seat", 19},
		{"code_assist_ent", "Gemini Code Assist Enterprise", "$45/mo per seat", 45},
		{"api", "API / pay-as-you-go", "", 0},
		{"skip", "(not set)", "", 0},
	},
}

// providerOrder is the fixed prompt order for configure's provider picker.
var providerOrder = []string{"claude", "codex", "gemini"}

var providerLabel = map[string]string{
	"claude": "Claude",
	"codex":  "Codex (ChatGPT)",
	"gemini": "Gemini",
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

// LoadConfig returns the stored config, or (nil, nil) if none exists yet. It
// transparently upgrades the old single-plan (Claude-only) file format,
// re-saving it under the new per-provider shape.
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
	if err := json.Unmarshal(b, &c); err == nil && c.Plans != nil {
		return &c, nil
	}

	// Legacy format: {"plan":"pro","plan_name":...,"price_label":...,"monthly_usd":...}
	// — implicitly a Claude plan, from before other providers were supported.
	var legacy struct {
		Plan       string  `json:"plan"`
		PlanName   string  `json:"plan_name"`
		PriceLabel string  `json:"price_label"`
		MonthlyUSD float64 `json:"monthly_usd"`
	}
	if err := json.Unmarshal(b, &legacy); err != nil {
		return nil, err
	}
	migrated := &Config{Plans: map[string]ProviderPlan{}}
	if legacy.Plan != "" {
		migrated.Plans["claude"] = ProviderPlan{
			Plan: legacy.Plan, PlanName: legacy.PlanName,
			PriceLabel: legacy.PriceLabel, MonthlyUSD: legacy.MonthlyUSD,
		}
	}
	_ = SaveConfig(migrated) // best-effort: upgrade the file on disk
	return migrated, nil
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

// ensureConfig returns the stored config, prompting once per provider in
// wanted that isn't already configured (e.g. only ask about Codex the first
// time a report actually has Codex spend in it — never for a provider the
// caller isn't using). Returns nil only when nothing at all is configured and
// there's nothing new to ask (a non-interactive context, or wanted is empty).
func ensureConfig(wanted []string) *Config {
	cfg, _ := LoadConfig()
	if cfg == nil {
		cfg = &Config{Plans: map[string]ProviderPlan{}}
	}

	var missing []string
	for _, p := range wanted {
		if _, ok := cfg.Plans[p]; !ok {
			missing = append(missing, p)
		}
	}
	if len(missing) == 0 || !stdinIsTTY() {
		if len(cfg.Plans) == 0 {
			return nil
		}
		return cfg
	}

	names := make([]string, len(missing))
	for i, p := range missing {
		names[i] = providerLabel[p]
	}
	fmt.Fprintf(os.Stderr, "First run for %s — let's set your plan so cost is framed against what you pay.\n", strings.Join(names, ", "))
	for _, p := range missing {
		if pp := promptProviderPlan(p); pp != nil {
			cfg.Plans[p] = *pp
		}
	}
	if err := SaveConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not save config: %v\n", err)
	} else {
		path, _ := configPath()
		fmt.Fprintf(os.Stderr, "saved to %s (run `costblame configure` to change)\n\n", path)
	}
	return cfg
}

// priceSuffix formats " (label)" for a menu line, or "" for a $0 entry
// (API/pay-as-you-go, Skip) that has no fixed price to show.
func priceSuffix(label string) string {
	if label == "" {
		return ""
	}
	return " (" + label + ")"
}

// promptProviderPlan renders one provider's plan menu and reads a choice.
func promptProviderPlan(provider string) *ProviderPlan {
	catalog := planCatalogs[provider]
	r := bufio.NewReader(os.Stdin)
	for attempts := 0; attempts < 5; attempts++ {
		fmt.Fprintf(os.Stderr, "\nWhat's your %s plan?\n", providerLabel[provider])
		for i, p := range catalog {
			fmt.Fprintf(os.Stderr, "  [%d] %s%s\n", i+1, p.name, priceSuffix(p.priceLabel))
		}
		fmt.Fprintf(os.Stderr, "Choice [1-%d]: ", len(catalog))

		line, err := r.ReadString('\n')
		choice := strings.TrimSpace(line)
		if (err != nil && choice == "") || choice == "" {
			return catalogPlan(provider, "skip") // last catalog entry is always "skip"
		}
		if idx, convErr := strconv.Atoi(choice); convErr == nil && idx >= 1 && idx <= len(catalog) {
			return catalogPlan(provider, catalog[idx-1].key)
		}
		fmt.Fprintf(os.Stderr, "  (enter a number 1-%d)\n", len(catalog))
	}
	return catalogPlan(provider, "skip")
}

func catalogPlan(provider, key string) *ProviderPlan {
	for _, p := range planCatalogs[provider] {
		if p.key == key {
			return &ProviderPlan{Plan: p.key, PlanName: p.name, PriceLabel: p.priceLabel, MonthlyUSD: p.usd}
		}
	}
	return &ProviderPlan{Plan: "skip", PlanName: "(not set)"}
}

// runConfigure is the `costblame configure` subcommand: a provider picker
// loop, so you can set a plan for one provider, several, or all three,
// re-running it anytime to change one.
func runConfigure(_ []string) {
	if !stdinIsTTY() {
		fatal("configure needs an interactive terminal")
	}
	cfg, _ := LoadConfig()
	if cfg == nil {
		cfg = &Config{Plans: map[string]ProviderPlan{}}
	}

	r := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprintln(os.Stderr, "\nWhich provider do you want to set a plan for?")
		for i, p := range providerOrder {
			cur := "not set"
			if pp, ok := cfg.Plans[p]; ok {
				cur = pp.PlanName
			}
			fmt.Fprintf(os.Stderr, "  [%d] %s — currently: %s\n", i+1, providerLabel[p], cur)
		}
		done := len(providerOrder) + 1
		fmt.Fprintf(os.Stderr, "  [%d] Done\n", done)
		fmt.Fprintf(os.Stderr, "Choice [1-%d]: ", done)

		line, err := r.ReadString('\n')
		choice := strings.TrimSpace(line)
		if (err != nil && choice == "") || choice == "" {
			break
		}
		idx, convErr := strconv.Atoi(choice)
		if convErr != nil || idx < 1 || idx > done {
			fmt.Fprintf(os.Stderr, "  (enter a number 1-%d)\n", done)
			continue
		}
		if idx == done {
			break
		}
		p := providerOrder[idx-1]
		if pp := promptProviderPlan(p); pp != nil {
			cfg.Plans[p] = *pp
		}
	}

	if err := SaveConfig(cfg); err != nil {
		fatal("saving config: %v", err)
	}
	path, _ := configPath()
	fmt.Fprintf(os.Stderr, "\nSaved to %s\n", path)
}

package main

import (
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed pricing.json
var defaultPricing []byte

// ModelRate holds per-million-token USD rates for one model-id prefix.
type ModelRate struct {
	Prefix       string  `json:"prefix"`
	Input        float64 `json:"input"`
	Output       float64 `json:"output"`
	CacheWrite   float64 `json:"cache_write"`    // 5-minute TTL (~1.25x input)
	CacheWrite1h float64 `json:"cache_write_1h"` // 1-hour TTL (~2x input)
	CacheRead    float64 `json:"cache_read"`
}

// PricingTable is the parsed pricing configuration.
type PricingTable struct {
	Models []ModelRate `json:"models"`
}

// LoadPricing resolves the pricing table in priority order:
//  1. explicit path (--pricing flag), if non-empty
//  2. pricing.json sitting next to the binary
//  3. the table embedded at build time
//
// The returned string names the source used, for reporting.
func LoadPricing(explicit string) (*PricingTable, string, error) {
	if explicit != "" {
		pt, err := loadPricingFile(explicit)
		return pt, explicit, err
	}
	if exe, err := os.Executable(); err == nil {
		beside := filepath.Join(filepath.Dir(exe), "pricing.json")
		if _, statErr := os.Stat(beside); statErr == nil {
			pt, err := loadPricingFile(beside)
			return pt, beside, err
		}
	}
	pt, err := parsePricing(defaultPricing)
	return pt, "embedded defaults", err
}

func loadPricingFile(path string) (*PricingTable, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parsePricing(b)
}

func parsePricing(b []byte) (*PricingTable, error) {
	var pt PricingTable
	if err := json.Unmarshal(b, &pt); err != nil {
		return nil, err
	}
	// Back-compat: a pricing.json predating the 1h split omits cache_write_1h.
	// Derive it from the 5m rate (2x input / 1.25x input = 1.6x) so old files
	// keep working instead of pricing 1h cache writes at $0.
	for i := range pt.Models {
		if pt.Models[i].CacheWrite1h == 0 && pt.Models[i].CacheWrite > 0 {
			pt.Models[i].CacheWrite1h = pt.Models[i].CacheWrite * 1.6
		}
	}
	// Longest prefix first so the most-specific rate wins on lookup.
	sort.SliceStable(pt.Models, func(i, j int) bool {
		return len(pt.Models[i].Prefix) > len(pt.Models[j].Prefix)
	})
	return &pt, nil
}

// Cost returns the USD cost of one usage record. found reports whether any
// model prefix matched; an unmatched model yields ($0, false) so the caller
// can warn rather than silently mis-bill.
func (pt *PricingTable) Cost(model string, in, out, cacheWrite5m, cacheWrite1h, cacheRead int) (cost float64, found bool) {
	for _, m := range pt.Models {
		if strings.HasPrefix(model, m.Prefix) {
			cost = float64(in)/1e6*m.Input +
				float64(out)/1e6*m.Output +
				float64(cacheWrite5m)/1e6*m.CacheWrite +
				float64(cacheWrite1h)/1e6*m.CacheWrite1h +
				float64(cacheRead)/1e6*m.CacheRead
			return cost, true
		}
	}
	return 0, false
}

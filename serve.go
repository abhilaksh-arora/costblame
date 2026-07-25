package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

//go:embed dashboard.html
var dashboardHTML []byte

// runServe starts a local dashboard. It rebuilds the report on each /api/report
// request so a refresh always reflects current logs. Nothing leaves the machine
// — it binds to localhost and serves only its own embedded HTML.
func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	var repos repoList
	fs.Var(&repos, "repo", "repo to scope to (repeatable: --repo a --repo b); defaults to the current directory")
	all := fs.Bool("all", false, "show every project under ~/.claude etc., not just this repo")
	pricingPath := fs.String("pricing", "", "path to a pricing.json overriding the embedded defaults")
	raw := fs.Bool("raw", false, "show raw API-equivalent cost only, without the plan comparison")
	port := fs.Int("port", 7777, "localhost port to bind")
	fs.Parse(args)

	pt, src, err := LoadPricing(*pricingPath)
	if err != nil {
		fatal("loading pricing: %v", err)
	}
	cfg := ensureConfig() // prompt once if interactive and unconfigured

	build := func() (Report, error) {
		var projects []ProjectData
		switch {
		case *all:
			ps, err := CollectAll()
			if err != nil {
				return Report{}, err
			}
			projects = ps
		case len(repos) > 0:
			for _, r := range repos {
				pd, err := CollectRepo(r)
				if err != nil {
					return Report{}, err
				}
				projects = append(projects, pd)
			}
		default:
			pd, err := CollectRepo(".")
			if err != nil {
				return Report{}, err
			}
			projects = []ProjectData{pd}
		}
		rep := BuildReport(projects, pt, src)
		attachPlan(&rep, cfg, *raw)
		return rep, nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(dashboardHTML)
	})
	mux.HandleFunc("/api/report", func(w http.ResponseWriter, r *http.Request) {
		rep, err := build()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(rep)
	})

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	scope := "all projects"
	switch {
	case *all:
		// scope already set
	case len(repos) > 0:
		scope = strings.Join([]string(repos), ", ")
	default:
		if abs, aerr := filepath.Abs("."); aerr == nil {
			scope = abs
		} else {
			scope = "."
		}
	}
	fmt.Fprintf(os.Stderr, "costblame dashboard (%s) → http://%s\n", scope, addr)
	fmt.Fprintln(os.Stderr, "serving on localhost only; Ctrl-C to stop")
	if err := http.ListenAndServe(addr, mux); err != nil {
		fatal("serve: %v", err)
	}
}

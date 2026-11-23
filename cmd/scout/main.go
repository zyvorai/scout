// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/zyvorai/scout/internal/diff"
	"github.com/zyvorai/scout/internal/discovery"
	"github.com/zyvorai/scout/internal/engine"
	"github.com/zyvorai/scout/internal/estate"
	"github.com/zyvorai/scout/internal/graph"
	"github.com/zyvorai/scout/internal/intake"
	"github.com/zyvorai/scout/internal/inventory"
	"github.com/zyvorai/scout/internal/model"
	"github.com/zyvorai/scout/internal/plan"
	"github.com/zyvorai/scout/internal/policy"
	"github.com/zyvorai/scout/internal/report"
	"github.com/zyvorai/scout/internal/server"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	var err error
	switch os.Args[1] {
	case "scan":
		err = cmdScan(os.Args[2:])
	case "import":
		err = cmdImport(os.Args[2:])
	case "diff":
		err = cmdDiff(os.Args[2:])
	case "plan":
		err = cmdPlan(os.Args[2:])
	case "policy":
		err = cmdPolicy(os.Args[2:])
	case "assess":
		err = cmdAssess(os.Args[2:])
	case "report":
		err = cmdReport(os.Args[2:])
	case "serve":
		err = cmdServe(os.Args[2:], logger)
	case "version", "--version", "-v":
		fmt.Printf("scout %s\n", version)
		return
	case "help", "--help", "-h":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		logger.Error("command failed", "error", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`Zyvor Scout — migration discovery and readiness

Usage:
  scout scan   --source demo --out scout.json
  scout scan   --source vmware --url https://vcenter.example --user admin --out scout.json
  scout import --rvtools rvtools.xlsx --storage storage.json --dr dr.yaml --assumptions assumptions.json --out scout.json
  scout import --csv inventory.csv | --scout-json inventory.json [--validate] --out scout.json
  scout assess --file scout.json [--format json|sarif|csv] [--policy policy.json [--fail-on-expired]]
  scout policy check policy.json        validate a scout-policy/1 file and list waivers that expire soon
  scout policy rules                    list the rule ids a policy may name
  scout diff   old.json new.json [--format text] [--fail-on regression,new-blocker,score-drop=5]
  scout plan   --file scout.json [--link-mbps 1000 --max-concurrent 4 --window 240 --exclusive "db-1,db-2"]
  scout plan   --file scout.json --validate plan.json [--move vm=2] [--format text]   (--draft writes an editable plan)
  scout report --file scout.json --out report.html --executive executive.html --pdf executive.pdf --workbook workbook.zip
  scout serve  --file scout.json --addr 127.0.0.1:18447
  scout version

Commands:
  scan     Discover or generate an inventory
  import   Read local RVTools, storage.json, and dr.yaml. Nothing is uploaded.
  assess   Print readiness assessments as JSON, SARIF 2.1.0, or findings CSV; --policy applies waivers and overrides
  policy   Validate a policy file (waivers need a reason and an expiry date) or list rule ids
  diff     Compare two assessments (or inventories): changed verdicts, rules that started or stopped firing, score deltas
  plan     Estimate downtime and maintenance windows per wave under constraints
  report   Write the HTML report, executive PDF, and CSV workbook
  serve    Start the embedded dashboard and JSON API (loopback by default)

Inputs stay on this machine. Reports are written mode 0600.
Environment for VMware discovery:
  VCENTER_URL, VCENTER_USERNAME, VCENTER_PASSWORD
`)
}

func cmdScan(args []string) error {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	source := fs.String("source", "demo", "discovery source: demo|vmware")
	out := fs.String("out", "scout.json", "output inventory JSON")
	url := fs.String("url", os.Getenv("VCENTER_URL"), "vCenter base URL")
	user := fs.String("user", os.Getenv("VCENTER_USERNAME"), "vCenter username")
	password := fs.String("password", os.Getenv("VCENTER_PASSWORD"), "vCenter password (prefer VCENTER_PASSWORD)")
	insecure := fs.Bool("insecure", false, "skip vCenter TLS certificate verification")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var d discovery.Discoverer
	switch strings.ToLower(*source) {
	case "demo":
		d = discovery.Demo{}
	case "vmware":
		if *url == "" || *user == "" || *password == "" {
			return fmt.Errorf("vmware source requires --url, --user and password (prefer VCENTER_PASSWORD)")
		}
		d = discovery.VMware{BaseURL: *url, Username: *user, Password: *password, Insecure: *insecure}
	default:
		return fmt.Errorf("unsupported source %q", *source)
	}
	inv, err := d.Discover(ctx)
	if err != nil {
		return err
	}
	if err := inventory.Save(*out, inv); err != nil {
		return err
	}
	fmt.Printf("Discovered %d VMs from %s -> %s\n", len(inv.VMs), inv.Source, *out)
	return nil
}

func cmdAssess(args []string) error {
	fs := flag.NewFlagSet("assess", flag.ContinueOnError)
	file := fs.String("file", "scout.json", "inventory JSON")
	format := fs.String("format", "json", "output format: json, sarif, or csv")
	policyFile := fs.String("policy", "", "scout-policy/1 file with waivers and rule overrides")
	failExpired := fs.Bool("fail-on-expired", false, "with --policy: exit 1 when a waiver has expired or matched nothing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	inv, err := inventory.Load(*file)
	if err != nil {
		return err
	}
	a := engine.Evaluate(inv)
	a, est := estate.Annotate(inv, a)
	var polReport *policy.Report
	if *policyFile != "" {
		now := time.Now()
		pol, err := policy.Load(*policyFile, now)
		if err != nil {
			return err
		}
		var rep policy.Report
		a, rep = policy.Apply(a, pol, now)
		polReport = &rep
		fmt.Fprint(os.Stderr, policySummary(rep))
		if *failExpired && (rep.Counts.ExpiredWaivers > 0 || rep.Counts.UnusedWaivers > 0) {
			defer func() { os.Exit(1) }()
		}
	}
	switch *format {
	case "json":
	case "sarif":
		return report.WriteSARIF(os.Stdout, version, a)
	case "csv":
		return report.WriteFindingsCSV(os.Stdout, a)
	default:
		return fmt.Errorf("unknown --format %q (want json, sarif, or csv)", *format)
	}
	out := map[string]any{"summary": engine.Summary(inv, a), "assessments": a, "graph": graph.Build(inv, a), "estate": est, "estateFindings": engine.EstateFindings(a)}
	if polReport != nil {
		out["policy"] = polReport
	}
	return writeJSON(os.Stdout, out)
}

// policySummary prints what a policy did to stderr, so a pipeline that reads
// JSON from stdout still shows a human the waivers and overrides in effect.
func policySummary(r policy.Report) string {
	c := r.Counts
	var b strings.Builder
	fmt.Fprintf(&b, "policy: %d finding(s) waived (%d blocker), %d dropped by disabled rules, %d severity/penalty change(s) (%d blocker lowered)\n", c.FindingsWaived, c.BlockersWaived, c.FindingsDropped, c.FindingsChanged, c.BlockersLowered)
	for _, e := range r.Expired {
		fmt.Fprintf(&b, "policy: EXPIRED waiver %s on %q (%s): %d finding(s) are active again\n", e.Rule, e.VM, e.Expires, e.Matched)
	}
	for _, u := range r.Unused {
		fmt.Fprintf(&b, "policy: waiver %s on %q matched no finding (stale or mistyped selector)\n", u.Rule, u.VM)
	}
	for _, e := range r.Expiring {
		fmt.Fprintf(&b, "policy: waiver %s on %q expires in %d day(s) (%s)\n", e.Rule, e.VM, e.InDays, e.Expires)
	}
	return b.String()
}

func cmdPolicy(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: scout policy check FILE | scout policy rules")
	}
	switch args[0] {
	case "rules":
		for _, id := range engine.KnownRuleIDs() {
			fmt.Println(id)
		}
		return nil
	case "check":
		if len(args) != 2 {
			return fmt.Errorf("usage: scout policy check FILE")
		}
		now := time.Now()
		pol, err := policy.Load(args[1], now)
		if err != nil {
			return err
		}
		fmt.Printf("ok: %s (%d rule override(s), %d waiver(s))\n", args[1], len(pol.Rules), len(pol.Waivers))
		_, rep := policy.Apply(nil, pol, now)
		for _, e := range rep.Expired {
			fmt.Printf("expired: %s on %q (%s)\n", e.Rule, e.VM, e.Expires)
		}
		for _, e := range rep.Expiring {
			fmt.Printf("expires in %d day(s): %s on %q (%s)\n", e.InDays, e.Rule, e.VM, e.Expires)
		}
		if len(rep.Expired) > 0 {
			return fmt.Errorf("%d waiver(s) have expired", len(rep.Expired))
		}
		return nil
	default:
		return fmt.Errorf("unknown policy command %q (want check or rules)", args[0])
	}
}

func cmdDiff(args []string) error {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	format := fs.String("format", "json", "output format: json or text")
	failOn := fs.String("fail-on", "", "exit 1 when: regression, new-blocker, score-drop=N (comma separated)")
	// Accept the two files before or after the flags.
	var files []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			break
		}
		files = append(files, fs.Arg(0))
		rest = fs.Args()[1:]
	}
	if len(files) != 2 {
		return fmt.Errorf("diff needs exactly two files: scout diff old.json new.json")
	}
	if *format != "json" && *format != "text" {
		return fmt.Errorf("unknown --format %q (want json or text)", *format)
	}
	load := func(path string) ([]model.Assessment, error) {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		a, err := diff.Load(f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return a, nil
	}
	oldA, err := load(files[0])
	if err != nil {
		return err
	}
	newA, err := load(files[1])
	if err != nil {
		return err
	}
	res := diff.Compare(oldA, newA)
	gate, err := diff.Evaluate(res, *failOn, newA)
	if err != nil {
		return err
	}
	if *format == "text" {
		fmt.Print(res.Text())
		for _, r := range gate.Reasons {
			fmt.Println("GATE FAILED: " + r)
		}
	} else if err := writeJSON(os.Stdout, map[string]any{"result": res, "gate": gate}); err != nil {
		return err
	}
	if gate.Failed {
		return fmt.Errorf("gate failed: %s", strings.Join(gate.Reasons, "; "))
	}
	return nil
}

func cmdPlan(args []string) error {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	file := fs.String("file", "scout.json", "inventory JSON")
	c := plan.Constraints{}
	fs.IntVar(&c.MaxConcurrent, "max-concurrent", 0, "VMs cut over in parallel (default 4)")
	fs.IntVar(&c.WindowMinutes, "window", 0, "maintenance window minutes (default 240)")
	fs.Float64Var(&c.LinkMbps, "link-mbps", 0, "usable link Mbps (default from assumptions)")
	fs.Float64Var(&c.Efficiency, "efficiency", 0, "achieved fraction of the link, 0-1 (default 0.7)")
	fs.IntVar(&c.CutoverMinutes, "cutover-minutes", 0, "per-VM cutover overhead (default from assumptions, else 15)")
	fs.Float64Var(&c.DeltaPercent, "delta-percent", 0, "final warm sync as percent of data (default 5)")
	exclusive := fs.String("exclusive", "", "VM ids or names that must not cut over together, comma separated; repeat groups with ';'")
	validate := fs.String("validate", "", "validate a user-edited wave plan (scout-plan/1 JSON); exits 1 on errors")
	draft := fs.Bool("draft", false, "print the automatic wave assignment as an editable scout-plan/1 file")
	format := fs.String("format", "json", "output format for -validate: json or text")
	var moves moveFlags
	fs.Var(&moves, "move", "what-if with -validate: move a VM to a wave, as VM=WAVE (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *format != "json" && *format != "text" {
		return fmt.Errorf("unknown --format %q (want json or text)", *format)
	}
	if len(moves) > 0 && *validate == "" {
		return fmt.Errorf("-move requires -validate")
	}
	inv, err := inventory.Load(*file)
	if err != nil {
		return err
	}
	base := plan.FromInventory(inv)
	if c.LinkMbps == 0 {
		c.LinkMbps = base.LinkMbps
	}
	if c.CutoverMinutes == 0 {
		c.CutoverMinutes = base.CutoverMinutes
	}
	for _, g := range strings.Split(*exclusive, ";") {
		var group []string
		for _, ref := range strings.Split(g, ",") {
			if ref = strings.TrimSpace(ref); ref != "" {
				group = append(group, ref)
			}
		}
		if len(group) > 1 {
			c.ExclusiveGroups = append(c.ExclusiveGroups, group)
		}
	}
	a := engine.Evaluate(inv)
	if *draft {
		return writeJSON(os.Stdout, plan.Draft(inv, a, c))
	}
	if *validate != "" {
		return runValidate(inv, a, *validate, *format, moves)
	}
	p, err := plan.Build(inv, a, c)
	if err != nil {
		return err
	}
	return writeJSON(os.Stdout, p)
}

// moveFlags collects repeated -move VM=WAVE values.
type moveFlags []plan.Move

func (m *moveFlags) String() string { return fmt.Sprint(len(*m)) }

func (m *moveFlags) Set(v string) error {
	i := strings.LastIndex(v, "=")
	if i <= 0 {
		return fmt.Errorf("move %q must look like VM=WAVE", v)
	}
	n, err := strconv.Atoi(strings.TrimSpace(v[i+1:]))
	if err != nil || n <= 0 {
		return fmt.Errorf("move %q: wave must be a positive integer", v)
	}
	*m = append(*m, plan.Move{VM: strings.TrimSpace(v[:i]), ToWave: n})
	return nil
}

var errPlanInvalid = fmt.Errorf("plan has errors")

func runValidate(inv model.Inventory, a []model.Assessment, path, format string, moves moveFlags) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open plan: %w", err)
	}
	defer f.Close()
	e, err := plan.ParseEdited(f)
	if err != nil {
		return err
	}
	if len(moves) > 0 {
		r, err := plan.WhatIf(inv, a, e, moves)
		if err != nil {
			return err
		}
		if format == "text" {
			fmt.Print(r.Text())
		} else if err := writeJSON(os.Stdout, r); err != nil {
			return err
		}
		if !r.After.OK {
			return errPlanInvalid
		}
		return nil
	}
	v := plan.Validate(inv, a, e)
	if format == "text" {
		fmt.Print(v.Text())
	} else if err := writeJSON(os.Stdout, v); err != nil {
		return err
	}
	if !v.OK {
		return errPlanInvalid
	}
	return nil
}

func cmdImport(args []string) error {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	rvtools := fs.String("rvtools", "", "local RVTools .xlsx export")
	file := fs.String("file", "", "existing inventory JSON to enrich")
	storage := fs.String("storage", "", "local storage.json")
	dr := fs.String("dr", "", "local dr.yaml")
	assumptions := fs.String("assumptions", "", "local assumptions.json")
	csvPath := fs.String("csv", "", "local CSV inventory (header contract in docs/IMPORT.md)")
	scoutJSON := fs.String("scout-json", "", "local scout-inventory/1 JSON file (schema in docs/scout-inventory-1.schema.json)")
	validate := fs.Bool("validate", false, "with --csv or --scout-json: check the file and report line-numbered problems; writes nothing, exits 1 on errors")
	environment := fs.String("environment", "", "environment name for --csv imports")
	out := fs.String("out", "scout.json", "output inventory JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	sources := 0
	for _, s := range []string{*rvtools, *file, *csvPath, *scoutJSON} {
		if s != "" {
			sources++
		}
	}
	if sources == 0 {
		return fmt.Errorf("import requires one of --rvtools, --csv, --scout-json or --file")
	}
	if sources > 1 {
		return fmt.Errorf("choose only one of --rvtools, --csv, --scout-json and --file")
	}
	if *validate && *csvPath == "" && *scoutJSON == "" {
		return fmt.Errorf("--validate supports --csv and --scout-json")
	}
	var inv model.Inventory
	var err error
	if *csvPath != "" || *scoutJSON != "" {
		path := *csvPath
		var issues []intake.Issue
		if *csvPath != "" {
			inv, issues, err = intake.FromCSVFile(path, time.Time{})
		} else {
			path = *scoutJSON
			inv, issues, err = intake.FromScoutJSONFile(path)
		}
		if err != nil {
			return err
		}
		for _, i := range issues {
			fmt.Fprintf(os.Stderr, "%s: %s\n", path, i)
		}
		if intake.HasErrors(issues) {
			return fmt.Errorf("%s has errors; nothing was written", path)
		}
		if *validate {
			fmt.Printf("%s: valid (%d VMs, %d warnings)\n", path, len(inv.VMs), len(issues))
			return nil
		}
		if *environment != "" {
			inv.Environment = *environment
		}
	} else if *rvtools != "" {
		inv, err = intake.FromRVTools(*rvtools, time.Time{})
		if err != nil {
			return err
		}
	} else {
		loaded, err := inventory.Load(*file)
		if err != nil {
			return err
		}
		inv = loaded
	}
	if *storage != "" {
		if err := intake.MergeStorage(&inv, *storage); err != nil {
			return err
		}
	}
	if *dr != "" {
		if err := intake.MergeDR(&inv, *dr); err != nil {
			return err
		}
	}
	if *assumptions != "" {
		if err := intake.MergeAssumptions(&inv, *assumptions); err != nil {
			return err
		}
	}
	if err := inventory.Save(*out, inv); err != nil {
		return err
	}
	fmt.Printf("Imported %d VMs -> %s (local file, mode 0600)\n", len(inv.VMs), *out)
	for _, w := range inv.ImportWarnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}
	return nil
}

func cmdReport(args []string) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	file := fs.String("file", "scout.json", "inventory JSON")
	out := fs.String("out", "", "technical HTML report")
	executive := fs.String("executive", "", "executive print HTML")
	pdfPath := fs.String("pdf", "", "executive PDF")
	workbook := fs.String("workbook", "", "technical workbook zip")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" && *executive == "" && *pdfPath == "" && *workbook == "" {
		*out = "report.html"
	}
	inv, err := inventory.Load(*file)
	if err != nil {
		return err
	}
	data := report.Build(inv)
	if *out != "" {
		if err := writePrivate(*out, func(w io.Writer) error { return report.Write(w, inv) }); err != nil {
			return err
		}
		fmt.Printf("Report written to %s\n", *out)
	}
	if *executive != "" {
		if err := writePrivate(*executive, func(w io.Writer) error { return report.WriteExecutive(w, data) }); err != nil {
			return err
		}
		fmt.Printf("Executive report written to %s\n", *executive)
	}
	if *pdfPath != "" {
		if err := writePrivate(*pdfPath, func(w io.Writer) error { return report.WritePDF(w, data) }); err != nil {
			return err
		}
		fmt.Printf("Executive PDF written to %s\n", *pdfPath)
	}
	if *workbook != "" {
		if err := writePrivate(*workbook, func(w io.Writer) error { return report.WriteWorkbook(w, data) }); err != nil {
			return err
		}
		fmt.Printf("Workbook written to %s\n", *workbook)
	}
	return nil
}

func writePrivate(path string, write func(io.Writer) error) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return write(f)
}

func cmdServe(args []string, logger *slog.Logger) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	file := fs.String("file", "scout.json", "inventory JSON")
	addr := fs.String("addr", "127.0.0.1:18447", "listen address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	inv, err := inventory.Load(*file)
	if err != nil {
		return err
	}
	server.Version = version
	return server.Listen(*addr, inv, logger)
}

func writeJSON(f *os.File, v any) error {
	// Kept here to make CLI output deterministic and dependency-free.
	b, err := marshalIndent(v)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}

func marshalIndent(v any) ([]byte, error) {
	// local wrapper avoids leaking encoding choices into command implementations
	return jsonMarshalIndent(v)
}

// split out for simple unit replacement if CLI formats are added later
var jsonMarshalIndent = func(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

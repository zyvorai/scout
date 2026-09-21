// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/zyvorai/scout/internal/discovery"
	"github.com/zyvorai/scout/internal/engine"
	"github.com/zyvorai/scout/internal/estate"
	"github.com/zyvorai/scout/internal/graph"
	"github.com/zyvorai/scout/internal/intake"
	"github.com/zyvorai/scout/internal/inventory"
	"github.com/zyvorai/scout/internal/model"
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
  scout assess --file scout.json
  scout report --file scout.json --out report.html --executive executive.html --pdf executive.pdf --workbook workbook.zip
  scout serve  --file scout.json --addr 127.0.0.1:18447
  scout version

Commands:
  scan     Discover or generate an inventory
  import   Read local RVTools, storage.json, and dr.yaml. Nothing is uploaded.
  assess   Print readiness assessments as JSON
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
	if err := fs.Parse(args); err != nil {
		return err
	}
	inv, err := inventory.Load(*file)
	if err != nil {
		return err
	}
	a := graph.AssignWaves(inv, engine.Assess(inv, nil))
	a, est := estate.Annotate(inv, a)
	return writeJSON(os.Stdout, map[string]any{"summary": engine.Summary(inv, a), "assessments": a, "graph": graph.Build(inv, a), "estate": est})
}

func cmdImport(args []string) error {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	rvtools := fs.String("rvtools", "", "local RVTools .xlsx export")
	file := fs.String("file", "", "existing inventory JSON to enrich")
	storage := fs.String("storage", "", "local storage.json")
	dr := fs.String("dr", "", "local dr.yaml")
	assumptions := fs.String("assumptions", "", "local assumptions.json")
	out := fs.String("out", "scout.json", "output inventory JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *rvtools == "" && *file == "" {
		return fmt.Errorf("import requires --rvtools or --file")
	}
	var inv model.Inventory
	var err error
	if *rvtools != "" {
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

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/zyvorai/scout/internal/discovery"
	"github.com/zyvorai/scout/internal/engine"
	"github.com/zyvorai/scout/internal/graph"
	"github.com/zyvorai/scout/internal/inventory"
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
  scout assess --file scout.json
  scout report --file scout.json --out report.html
  scout serve  --file scout.json --addr 127.0.0.1:18447
  scout version

Commands:
  scan     Discover or generate an inventory
  assess   Print readiness assessments as JSON
  report   Generate a portable HTML report
  serve    Start the embedded Zyvor-branded dashboard and JSON API

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
	return writeJSON(os.Stdout, map[string]any{"summary": engine.Summary(inv, a), "assessments": a, "graph": graph.Build(inv, a)})
}

func cmdReport(args []string) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	file := fs.String("file", "scout.json", "inventory JSON")
	out := fs.String("out", "report.html", "output HTML report")
	if err := fs.Parse(args); err != nil {
		return err
	}
	inv, err := inventory.Load(*file)
	if err != nil {
		return err
	}
	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := report.Write(f, inv); err != nil {
		return err
	}
	fmt.Printf("Report written to %s\n", *out)
	return nil
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

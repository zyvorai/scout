// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/zyvorai/scout/internal/engine"
	"github.com/zyvorai/scout/internal/estate"
	"github.com/zyvorai/scout/internal/graph"
	"github.com/zyvorai/scout/internal/inventory"
	"github.com/zyvorai/scout/internal/model"
	"github.com/zyvorai/scout/internal/plan"
	"github.com/zyvorai/scout/internal/report"
)

//go:embed static/*
var staticFS embed.FS

// Version is reported in SARIF output; the CLI sets it at start-up.
var Version = "dev"

type Server struct {
	Inventory model.Inventory
	Logger    *slog.Logger
}

func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()
	assessments := engine.Evaluate(s.Inventory)
	assessments, est := estate.Annotate(s.Inventory, assessments)
	summary := engine.Summary(s.Inventory, assessments)
	depGraph := graph.Build(s.Inventory, assessments)
	estateFindings := engine.EstateFindings(assessments)

	mux.HandleFunc("GET /api/v1/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "time": time.Now().UTC()})
	})
	mux.HandleFunc("GET /api/v1/inventory", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, s.Inventory) })
	mux.HandleFunc("GET /api/v1/assessments", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, assessments) })
	mux.HandleFunc("GET /api/v1/summary", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, summary) })
	mux.HandleFunc("GET /api/v1/estate", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, est) })
	mux.HandleFunc("GET /api/v1/estate/findings", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, estateFindings) })
	mux.HandleFunc("GET /api/v1/graph", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, depGraph) })
	mux.HandleFunc("GET /api/v1/report", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Disposition", `inline; filename="scout-report.html"`)
		if err := report.Write(w, s.Inventory); err != nil && s.Logger != nil {
			s.Logger.Error("report", "error", err)
		}
	})
	mux.HandleFunc("GET /api/v1/findings.csv", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="scout-findings.csv"`)
		_ = report.WriteFindingsCSV(w, assessments)
	})
	mux.HandleFunc("GET /api/v1/sarif", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/sarif+json")
		w.Header().Set("Content-Disposition", `attachment; filename="scout.sarif"`)
		_ = report.WriteSARIF(w, Version, assessments)
	})
	mux.HandleFunc("GET /api/v1/plan", func(w http.ResponseWriter, r *http.Request) {
		c := plan.FromInventory(s.Inventory)
		q := r.URL.Query()
		for key, dst := range map[string]*float64{"linkMbps": &c.LinkMbps, "efficiency": &c.Efficiency, "deltaPercent": &c.DeltaPercent} {
			if v := q.Get(key); v != "" {
				f, err := strconv.ParseFloat(v, 64)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": key + " must be a number"})
					return
				}
				*dst = f
			}
		}
		for key, dst := range map[string]*int{"maxConcurrent": &c.MaxConcurrent, "windowMinutes": &c.WindowMinutes, "cutoverMinutes": &c.CutoverMinutes} {
			if v := q.Get(key); v != "" {
				n, err := strconv.Atoi(v)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": key + " must be an integer"})
					return
				}
				*dst = n
			}
		}
		writePlan(w, s.Inventory, assessments, c)
	})
	mux.HandleFunc("POST /api/v1/plan", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		defer r.Body.Close()
		c := plan.FromInventory(s.Inventory)
		var in plan.Constraints
		if err := decodeStrict(r.Body, &in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		// Request values override assumptions; zero values keep them.
		if in.MaxConcurrent != 0 {
			c.MaxConcurrent = in.MaxConcurrent
		}
		if in.WindowMinutes != 0 {
			c.WindowMinutes = in.WindowMinutes
		}
		if in.LinkMbps != 0 {
			c.LinkMbps = in.LinkMbps
		}
		if in.Efficiency != 0 {
			c.Efficiency = in.Efficiency
		}
		if in.CutoverMinutes != 0 {
			c.CutoverMinutes = in.CutoverMinutes
		}
		if in.DeltaPercent != 0 {
			c.DeltaPercent = in.DeltaPercent
		}
		c.ExclusiveGroups = in.ExclusiveGroups
		writePlan(w, s.Inventory, assessments, c)
	})
	mux.HandleFunc("GET /api/v1/plan/draft", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, plan.Draft(s.Inventory, assessments, plan.FromInventory(s.Inventory)))
	})
	mux.HandleFunc("POST /api/v1/plan/validate", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, plan.MaxEditedBytes)
		defer r.Body.Close()
		var in struct {
			Plan  plan.Edited `json:"plan"`
			Moves []plan.Move `json:"moves"`
		}
		if err := decodeStrict(r.Body, &in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if in.Plan.Schema != "" && in.Plan.Schema != plan.EditedSchema {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported plan schema " + in.Plan.Schema})
			return
		}
		if len(in.Moves) > 0 {
			res, err := plan.WhatIf(s.Inventory, assessments, in.Plan, in.Moves)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"validation": res.After, "whatIf": res})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"validation": plan.Validate(s.Inventory, assessments, in.Plan)})
	})
	mux.HandleFunc("POST /api/v1/analyze", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
		defer r.Body.Close()
		var inv model.Inventory
		if err := decodeStrict(r.Body, &inv); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := inventory.Validate(inv); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		a := engine.Evaluate(inv)
		a, est := estate.Annotate(inv, a)
		writeJSON(w, http.StatusOK, map[string]any{"summary": engine.Summary(inv, a), "assessments": a, "graph": graph.Build(inv, a), "estate": est, "estateFindings": engine.EstateFindings(a)})
	})

	web, _ := fs.Sub(staticFS, "static")
	fileServer := http.FileServer(http.FS(web))
	mux.Handle("GET /", fileServer)
	return securityHeaders(mux)
}

func (s Server) HTTP(addr string) *http.Server {
	return &http.Server{Addr: addr, Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
}

// decodeStrict decodes one JSON value, rejecting unknown fields and trailing data.
func decodeStrict(r io.Reader, v any) error {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return fmt.Errorf("unexpected data after the JSON document")
	}
	return nil
}

func writePlan(w http.ResponseWriter, inv model.Inventory, a []model.Assessment, c plan.Constraints) {
	p, err := plan.Build(inv, a, c)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; img-src 'self' data:; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}

func Listen(addr string, inv model.Inventory, logger *slog.Logger) error {
	s := Server{Inventory: inv, Logger: logger}.HTTP(addr)
	if logger != nil {
		logger.Info("Scout dashboard listening", "addr", addr)
	}
	if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

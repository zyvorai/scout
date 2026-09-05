package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/zyvorai/scout/internal/engine"
	"github.com/zyvorai/scout/internal/graph"
	"github.com/zyvorai/scout/internal/inventory"
	"github.com/zyvorai/scout/internal/model"
	"github.com/zyvorai/scout/internal/report"
)

//go:embed static/*
var staticFS embed.FS

type Server struct {
	Inventory model.Inventory
	Logger    *slog.Logger
}

func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()
	assessments := graph.AssignWaves(s.Inventory, engine.Assess(s.Inventory, nil))
	summary := engine.Summary(s.Inventory, assessments)
	depGraph := graph.Build(s.Inventory, assessments)

	mux.HandleFunc("GET /api/v1/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "time": time.Now().UTC()})
	})
	mux.HandleFunc("GET /api/v1/inventory", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, s.Inventory) })
	mux.HandleFunc("GET /api/v1/assessments", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, assessments) })
	mux.HandleFunc("GET /api/v1/summary", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, summary) })
	mux.HandleFunc("GET /api/v1/graph", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, depGraph) })
	mux.HandleFunc("GET /api/v1/report", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Disposition", `inline; filename="scout-report.html"`)
		if err := report.Write(w, s.Inventory); err != nil && s.Logger != nil {
			s.Logger.Error("report", "error", err)
		}
	})
	mux.HandleFunc("POST /api/v1/analyze", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
		defer r.Body.Close()
		var inv model.Inventory
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&inv); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := inventory.Validate(inv); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		a := graph.AssignWaves(inv, engine.Assess(inv, nil))
		writeJSON(w, http.StatusOK, map[string]any{"summary": engine.Summary(inv, a), "assessments": a, "graph": graph.Build(inv, a)})
	})

	web, _ := fs.Sub(staticFS, "static")
	fileServer := http.FileServer(http.FS(web))
	mux.Handle("GET /", fileServer)
	return securityHeaders(mux)
}

func (s Server) HTTP(addr string) *http.Server {
	return &http.Server{Addr: addr, Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
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

// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"sort"
	"time"

	"github.com/zyvorai/scout/internal/engine"
	"github.com/zyvorai/scout/internal/graph"
	"github.com/zyvorai/scout/internal/model"
)

type Data struct {
	Inventory   model.Inventory
	Assessments []model.Assessment
	Summary     model.Summary
	Generated   string
	JSON        template.JS
}

func Build(inv model.Inventory) Data {
	a := engine.Assess(inv, nil)
	a = graph.AssignWaves(inv, a)
	s := engine.Summary(inv, a)
	payload, _ := json.Marshal(struct {
		Inventory   model.Inventory    `json:"inventory"`
		Assessments []model.Assessment `json:"assessments"`
		Summary     model.Summary      `json:"summary"`
	}{inv, a, s})
	return Data{Inventory: inv, Assessments: a, Summary: s, Generated: time.Now().UTC().Format(time.RFC3339), JSON: template.JS(payload)}
}

func Write(w io.Writer, inv model.Inventory) error {
	data := Build(inv)
	sort.SliceStable(data.Assessments, func(i, j int) bool { return data.Assessments[i].Score < data.Assessments[j].Score })
	t, err := template.New("report").Parse(reportHTML)
	if err != nil {
		return fmt.Errorf("parse report template: %w", err)
	}
	if err := t.Execute(w, data); err != nil {
		return fmt.Errorf("render report: %w", err)
	}
	return nil
}

const reportHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Zyvor Scout — Migration Readiness Report</title>
<style>
:root{--bg:#f7f4f1;--card:#fff;--text:#1a1a1f;--muted:#6b6560;--accent:#cc420a;--fill:#ff5a15;--green:#16833a;--amber:#9a6700;--red:#b42318;--line:#e5ddd6}*{box-sizing:border-box}body{margin:0;background:radial-gradient(ellipse 70% 40% at 100% 0,rgba(255,90,21,.12),transparent 55%),var(--bg);font-family:-apple-system,BlinkMacSystemFont,"SF Pro Text","Segoe UI",sans-serif;color:var(--text)}main{max-width:1180px;margin:auto;padding:72px 28px 100px}.brand{display:flex;align-items:center;gap:12px;margin-bottom:28px}.brand svg{display:block;border-radius:10px;box-shadow:0 6px 18px rgba(255,90,21,.28)}.eyebrow{font-weight:800;color:var(--accent);letter-spacing:.12em;text-transform:uppercase;font-size:13px}h1{font-size:clamp(44px,7vw,78px);letter-spacing:-.055em;line-height:.95;margin:14px 0 18px}.sub{max-width:760px;color:var(--muted);font-size:22px;line-height:1.45}.grid{display:grid;grid-template-columns:repeat(5,1fr);gap:14px;margin:42px 0}.metric,.card{background:var(--card);border-radius:24px;padding:24px;box-shadow:0 1px 0 rgba(0,0,0,.04);border:1px solid #efe8e2}.metric b{font-size:36px;letter-spacing:-.04em;display:block}.metric span{color:var(--muted)}h2{font-size:34px;letter-spacing:-.035em;margin:60px 0 20px}.row{display:grid;grid-template-columns:1.5fr .7fr .8fr 2fr;gap:16px;align-items:center;padding:17px 4px;border-bottom:1px solid var(--line)}.row:last-child{border:0}.name{font-weight:700}.pill{display:inline-block;border-radius:999px;padding:7px 11px;font-size:12px;font-weight:800;text-transform:uppercase;letter-spacing:.03em}.ready{background:#e7f7ed;color:var(--green)}.review{background:#fff4d6;color:var(--amber)}.blocked{background:#feeceb;color:var(--red)}.score{font-weight:800;font-size:22px;color:var(--accent)}.findings{color:var(--muted);font-size:14px;line-height:1.45}.foot{margin-top:50px;color:var(--muted);font-size:13px}@media(max-width:800px){.grid{grid-template-columns:repeat(2,1fr)}.row{grid-template-columns:1fr .5fr}.row>div:nth-child(3),.row>div:nth-child(4){grid-column:1/-1}main{padding:44px 18px}}
</style></head><body><main>
<div class="brand" aria-label="Zyvor"><svg xmlns="http://www.w3.org/2000/svg" width="40" height="40" viewBox="0 0 64 64" role="img"><defs><linearGradient id="bg" x1="8" y1="8" x2="56" y2="56" gradientUnits="userSpaceOnUse"><stop offset="0" stop-color="#ff5a15"/><stop offset="1" stop-color="#cc420a"/></linearGradient></defs><rect width="64" height="64" rx="15" fill="url(#bg)"/><path d="M18.5,20.5 45.5,20.5 18.5,43.5 45.5,43.5" fill="none" stroke="#ffffff" stroke-width="8.3" stroke-linecap="round" stroke-linejoin="round"/></svg></div>
<div class="eyebrow">ZYVOR SCOUT</div><h1>Migration readiness.<br>Before migration risk.</h1><div class="sub">Assessment for <strong>{{.Inventory.Environment}}</strong>. Scout evaluates VM compatibility, flags blockers, and groups connected workloads into migration waves.</div>
<div class="grid"><div class="metric"><b>{{.Summary.TotalVMs}}</b><span>Virtual machines</span></div><div class="metric"><b>{{.Summary.Ready}}</b><span>Ready</span></div><div class="metric"><b>{{.Summary.Review}}</b><span>Review</span></div><div class="metric"><b>{{.Summary.Blocked}}</b><span>Blocked</span></div><div class="metric"><b>{{.Summary.AverageScore}}%</b><span>Average score</span></div></div>
<h2>Readiness by workload</h2><div class="card">{{range .Assessments}}<div class="row"><div><div class="name">{{.VMName}}</div><div style="color:#6e6e73;font-size:13px">{{.VMID}} · Wave {{.Wave}}</div></div><div class="score">{{.Score}}%</div><div><span class="pill {{.Status}}">{{.Status}}</span></div><div class="findings">{{if .Findings}}{{range $i,$f := .Findings}}{{if $i}} · {{end}}{{$f.Title}}{{end}}{{else}}No compatibility findings.{{end}}</div></div>{{end}}</div>
<div class="foot">Generated {{.Generated}} · Source: {{.Inventory.Source}} · Zyvor Scout does not modify source workloads.</div>
</main></body></html>`

// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"fmt"
	"html/template"
	"io"
)

func WriteExecutive(w io.Writer, invData Data) error {
	t, err := template.New("executive").Parse(executiveHTML)
	if err != nil {
		return fmt.Errorf("parse executive template: %w", err)
	}
	if err := t.Execute(w, invData); err != nil {
		return fmt.Errorf("render executive report: %w", err)
	}
	return nil
}

const executiveHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<title>Zyvor infrastructure assessment</title>
<style>
body{margin:0;font-family:Georgia,"Iowan Old Style",serif;color:#1a1a1f;background:#fff}
main{max-width:820px;margin:auto;padding:48px 40px 72px}
h1{font-size:32px;letter-spacing:-.03em;margin:0 0 8px}
h2{font-size:18px;margin:28px 0 8px}
p,li{line-height:1.45}
.meta{color:#5c564f;font-size:14px}
table{width:100%;border-collapse:collapse;font-size:14px;margin-top:8px}
th,td{text-align:left;border-bottom:1px solid #e5ddd6;padding:6px 4px;vertical-align:top}
.note{font-size:13px;color:#5c564f}
@media print { main{padding:0} }
</style></head><body><main>
<p class="meta">Zyvor infrastructure assessment · local report · {{.Generated}}</p>
<h1>{{.Inventory.Environment}}</h1>
<p>{{.Summary.TotalVMs}} virtual machines. {{.Summary.Ready}} ready, {{.Summary.Review}} need review, {{.Summary.Blocked}} blocked. Average compatibility score {{.Summary.AverageScore}}. Source: {{.Inventory.Source}}.</p>
<h2>Target architecture</h2>
<p>{{.Estate.Architecture}}</p>
<h2>Snapshots</h2>
<p>{{.Estate.SnapshotWaste.Detail}} <span class="note">({{.Estate.SnapshotWaste.Basis}})</span></p>
<h2>Capacity runway</h2>
<table><thead><tr><th>Source</th><th>Free</th><th>Free percent</th><th>Runway</th><th>Basis</th></tr></thead><tbody>
{{range .Estate.Runway}}<tr><td>{{.Source}}</td><td>{{.FreeBytes}}</td><td>{{.FreePercent}}</td><td>{{.Days}}</td><td>{{.Basis}}</td></tr>{{end}}
</tbody></table>
<h2>DR readiness</h2>
<p>Score {{.Estate.DR.Score}}. {{.Estate.DR.Detail}} <span class="note">({{.Estate.DR.Basis}})</span></p>
<h2>Migration waves and estimated downtime</h2>
<table><thead><tr><th>VM</th><th>Wave</th><th>Status</th><th>Estimated cutover minutes</th><th>Basis</th></tr></thead><tbody>
{{range .Assessments}}<tr><td>{{.VMName}}</td><td>{{.Wave}}</td><td>{{.Status}}</td><td>{{.EstimatedCutoverMinutes}}</td><td>{{.CutoverBasis}}</td></tr>{{end}}
</tbody></table>
<p class="note">Cutover minutes are disk bytes divided by the link rate you supplied, plus a fixed cutover window. They are an estimate, not a measured outage.</p>
<h2>Three-year figures</h2>
{{if .Estate.TCO.Lines}}<table><thead><tr><th>Line</th><th>Amount</th><th>Basis</th></tr></thead><tbody>
{{range .Estate.TCO.Lines}}<tr><td>{{.Label}}</td><td>{{.Amount}}</td><td>{{.Basis}}</td></tr>{{end}}
<tr><td>Customer three-year total</td><td>{{.Estate.TCO.Customer}}</td><td>measured inputs</td></tr>
<tr><td>Zyvor three-year total</td><td>{{.Estate.TCO.Zyvor}}</td><td>estimated</td></tr>
<tr><td>Arithmetic difference</td><td>{{.Estate.TCO.Delta}}</td><td>estimated</td></tr>
</tbody></table>{{end}}
{{range .Estate.TCO.Notes}}<p class="note">{{.}}</p>{{end}}
{{if .Inventory.ImportWarnings}}<h2>Import warnings</h2><ul>{{range .Inventory.ImportWarnings}}<li>{{.}}</li>{{end}}</ul>{{end}}
<p class="note">Scout does not modify source workloads. This file was produced locally and was not uploaded.</p>
</main></body></html>`

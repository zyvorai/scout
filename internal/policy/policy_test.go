package policy

import (
	"strings"
	"testing"
	"time"

	"github.com/zyvorai/scout/internal/engine"
	"github.com/zyvorai/scout/internal/model"
)

var now = time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)

func rid(t *testing.T) string {
	t.Helper()
	ids := engine.KnownRuleIDs()
	if len(ids) < 5 {
		t.Fatalf("expected the built-in rule ids, got %v", ids)
	}
	return "encrypted-vm"
}

func assessments() []model.Assessment {
	a := model.Assessment{VMID: "vm-1", VMName: "db-prod-01", Findings: []model.Finding{
		{RuleID: "encrypted-vm", Severity: model.SeverityBlocker, Title: "Encrypted VM", Penalty: 30},
		{RuleID: "rdm-disk", Severity: model.SeverityBlocker, Title: "RDM", Penalty: 40},
	}}
	b := model.Assessment{VMID: "vm-2", VMName: "web-01", Findings: []model.Finding{
		{RuleID: "encrypted-vm", Severity: model.SeverityBlocker, Title: "Encrypted VM", Penalty: 30},
	}}
	engine.Rescore(&a)
	engine.Rescore(&b)
	return []model.Assessment{a, b}
}

func mustParse(t *testing.T, s string) Policy {
	t.Helper()
	p, err := Parse([]byte(s), now)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return p
}

func TestWaiverRemovesFromScoreButStaysVisible(t *testing.T) {
	rid(t)
	p := mustParse(t, `{"schema":"scout-policy/1","waivers":[{"rule":"encrypted-vm","vm":"web-*","reason":"Vendor appliance, decrypted at cutover","owner":"ops","expires":"2027-01-31"}]}`)
	in := assessments()
	out, rep := Apply(in, p, now)
	if out[1].Status != "ready" || out[1].Score != 100 {
		t.Fatalf("web-01 should be ready/100 after the waiver, got %s/%d", out[1].Status, out[1].Score)
	}
	if len(out[1].Findings) != 0 || len(out[1].Waived) != 1 || out[1].Waived[0].WaivedBy != "ops" || out[1].Waived[0].Expires != "2027-01-31" {
		t.Fatalf("waived finding must stay visible: %+v", out[1])
	}
	if out[0].Status != "blocked" {
		t.Fatalf("db-prod-01 does not match web-*, must stay blocked: %s", out[0].Status)
	}
	if rep.Counts.FindingsWaived != 1 || rep.Counts.BlockersWaived != 1 || len(rep.Waived) != 1 {
		t.Fatalf("report: %+v", rep.Counts)
	}
	if len(in[1].Findings) != 1 || len(in[1].Waived) != 0 {
		t.Fatal("input assessments must not be modified")
	}
}

func TestExpiredWaiverNoLongerApplies(t *testing.T) {
	p := mustParse(t, `{"schema":"scout-policy/1","waivers":[{"rule":"encrypted-vm","vm":"web-01","reason":"Temporary exception for the pilot","expires":"2026-11-20"}]}`)
	out, rep := Apply(assessments(), p, now)
	if out[1].Status != "blocked" || len(out[1].Waived) != 0 {
		t.Fatalf("expired waiver must not apply: %s waived=%d", out[1].Status, len(out[1].Waived))
	}
	if len(rep.Expired) != 1 || rep.Expired[0].Matched != 1 || rep.Counts.ExpiredWaivers != 1 {
		t.Fatalf("expired report: %+v", rep.Expired)
	}
}

func TestWaiverActiveOnItsLastDay(t *testing.T) {
	p := mustParse(t, `{"schema":"scout-policy/1","waivers":[{"rule":"encrypted-vm","vm":"web-01","reason":"Last day of the exception","expires":"2026-11-23"}]}`)
	out, _ := Apply(assessments(), p, now)
	if len(out[1].Waived) != 1 {
		t.Fatal("a waiver is valid through its expiry date")
	}
}

func TestUnusedAndExpiringReported(t *testing.T) {
	p := mustParse(t, `{"schema":"scout-policy/1","waivers":[
	 {"rule":"encrypted-vm","vm":"no-such-vm","reason":"Stale selector left behind","expires":"2027-01-01"},
	 {"rule":"encrypted-vm","vm":"web-01","reason":"Expires soon, review it","expires":"2026-12-05"}]}`)
	_, rep := Apply(assessments(), p, now)
	if len(rep.Unused) != 1 || rep.Unused[0].VM != "no-such-vm" {
		t.Fatalf("unused: %+v", rep.Unused)
	}
	if len(rep.Expiring) != 1 || rep.Expiring[0].InDays != 12 {
		t.Fatalf("expiring: %+v", rep.Expiring)
	}
}

func TestDisableAndSeverityOverrideAreReported(t *testing.T) {
	p := mustParse(t, `{"schema":"scout-policy/1","rules":{
	  "rdm-disk":{"severity":"warning","reason":"RDMs are migrated with a separate tool here"},
	  "encrypted-vm":{"disabled":true,"reason":"Encryption handled by the platform team"}}}`)
	out, rep := Apply(assessments(), p, now)
	if out[0].Status != "review" {
		t.Fatalf("rdm-disk lowered to warning and encrypted-vm disabled should leave review, got %s", out[0].Status)
	}
	if rep.Counts.FindingsDropped != 2 || rep.Counts.BlockersLowered != 1 {
		t.Fatalf("counts: %+v", rep.Counts)
	}
	if len(rep.Disabled) != 1 || rep.Disabled[0].Reason == "" || len(rep.SeverityChange) != 1 || rep.SeverityChange[0].From != model.SeverityBlocker {
		t.Fatalf("report: %+v %+v", rep.Disabled, rep.SeverityChange)
	}
}

func TestPenaltyOverrideRescores(t *testing.T) {
	p := mustParse(t, `{"schema":"scout-policy/1","rules":{"rdm-disk":{"penalty":10}}}`)
	out, _ := Apply(assessments(), p, now)
	if out[0].Score != 100-30-10 {
		t.Fatalf("score %d", out[0].Score)
	}
}

func TestValidationRejects(t *testing.T) {
	cases := map[string]string{
		"wrong schema":        `{"schema":"x"}`,
		"unknown field":       `{"schema":"scout-policy/1","bogus":1}`,
		"unknown rule":        `{"schema":"scout-policy/1","rules":{"no-such-rule":{"disabled":true,"reason":"a long enough reason"}}}`,
		"waiver unknown rule": `{"schema":"scout-policy/1","waivers":[{"rule":"nope","vm":"a","reason":"a long enough reason","expires":"2026-12-01"}]}`,
		"no expiry":           `{"schema":"scout-policy/1","waivers":[{"rule":"encrypted-vm","vm":"a","reason":"a long enough reason"}]}`,
		"expiry too far":      `{"schema":"scout-policy/1","waivers":[{"rule":"encrypted-vm","vm":"a","reason":"a long enough reason","expires":"2030-01-01"}]}`,
		"short reason":        `{"schema":"scout-policy/1","waivers":[{"rule":"encrypted-vm","vm":"a","reason":"ok","expires":"2026-12-01"}]}`,
		"star selector":       `{"schema":"scout-policy/1","waivers":[{"rule":"encrypted-vm","vm":"*","reason":"a long enough reason","expires":"2026-12-01"}]}`,
		"bad pattern":         `{"schema":"scout-policy/1","waivers":[{"rule":"encrypted-vm","vm":"[","reason":"a long enough reason","expires":"2026-12-01"}]}`,
		"disable no reason":   `{"schema":"scout-policy/1","rules":{"encrypted-vm":{"disabled":true}}}`,
		"empty override":      `{"schema":"scout-policy/1","rules":{"encrypted-vm":{}}}`,
		"disabled and sev":    `{"schema":"scout-policy/1","rules":{"encrypted-vm":{"disabled":true,"severity":"info","reason":"a long enough reason"}}}`,
		"bad severity":        `{"schema":"scout-policy/1","rules":{"encrypted-vm":{"severity":"fatal","reason":"a long enough reason"}}}`,
		"penalty range":       `{"schema":"scout-policy/1","rules":{"encrypted-vm":{"penalty":101}}}`,
		"control char":        "{\"schema\":\"scout-policy/1\",\"waivers\":[{\"rule\":\"encrypted-vm\",\"vm\":\"a\",\"reason\":\"line one\\nforged line\",\"expires\":\"2026-12-01\"}]}",
		"trailing data":       `{"schema":"scout-policy/1"} {"x":1}`,
		"not json":            `schema: scout-policy/1`,
	}
	for name, body := range cases {
		if _, err := Parse([]byte(body), now); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestValidationListsEveryProblem(t *testing.T) {
	_, err := Parse([]byte(`{"schema":"scout-policy/1","waivers":[
	 {"rule":"nope","vm":"a","reason":"x","expires":"never"}]}`), now)
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"unknown rule id", "reason of at least", "expires must be a date"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q: %v", want, err)
		}
	}
}

func TestOversizeAndHostile(t *testing.T) {
	if _, err := Parse(make([]byte, MaxFileBytes+1), now); err == nil {
		t.Fatal("oversize policy must be rejected")
	}
	var b strings.Builder
	b.WriteString(`{"schema":"scout-policy/1","waivers":[`)
	for i := 0; i < MaxWaivers+1; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"rule":"encrypted-vm","vm":"a","reason":"a long enough reason","expires":"2026-12-01"}`)
	}
	b.WriteString(`]}`)
	if _, err := Parse([]byte(b.String()), now); err == nil || !strings.Contains(err.Error(), "more than") {
		t.Fatalf("too many waivers must be rejected: %v", err)
	}
}

func TestDeterministic(t *testing.T) {
	p := mustParse(t, `{"schema":"scout-policy/1","rules":{"rdm-disk":{"severity":"warning","reason":"separate tool migrates RDMs"}},"waivers":[{"rule":"encrypted-vm","vm":"*-01","reason":"covered by the pilot plan","expires":"2027-01-01"}]}`)
	a1, r1 := Apply(assessments(), p, now)
	a2, r2 := Apply(assessments(), p, now)
	if len(a1) != len(a2) || r1.Counts != r2.Counts || len(r1.Waived) != len(r2.Waived) {
		t.Fatal("Apply must be deterministic")
	}
}

func TestNoPolicyIsIdentity(t *testing.T) {
	in := assessments()
	out, rep := Apply(in, Policy{Schema: Schema}, now)
	for i := range in {
		if out[i].Score != in[i].Score || out[i].Status != in[i].Status || len(out[i].Findings) != len(in[i].Findings) {
			t.Fatal("an empty policy must not change anything")
		}
	}
	if rep.Counts != (Counts{}) {
		t.Fatalf("counts %+v", rep.Counts)
	}
}

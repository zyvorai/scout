// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package server

import (
	"regexp"
	"strings"
	"testing"
)

// Every element id the dashboard script looks up must exist in the page, and
// the script must not inject server data without the escaping helper.
func TestDashboardWiring(t *testing.T) {
	html, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	js, err := staticFS.ReadFile("static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range regexp.MustCompile(`\$\('([A-Za-z]+)'\)`).FindAllStringSubmatch(string(js), -1) {
		if !strings.Contains(string(html), `id="`+m[1]+`"`) {
			t.Errorf("app.js uses #%s, which index.html lacks", m[1])
		}
	}
	for _, id := range []string{"estate", "plancheck", "diff"} {
		if !strings.Contains(string(html), `href="#`+id+`"`) || !strings.Contains(string(html), `id="`+id+`"`) {
			t.Errorf("section %s missing from nav or page", id)
		}
	}
	// No inline scripts or handlers: the CSP allows script-src 'self' only.
	if regexp.MustCompile(`(?i)<script(\s[^>]*)?>[^<]`).MatchString(string(html)) || regexp.MustCompile(`(?i)\son[a-z]+=`).MatchString(string(html)) {
		t.Error("index.html must not contain inline script or event handlers")
	}
	// Raw ${...} interpolation of fields known to carry user data must go through esc().
	for _, bad := range []string{"${v.detail}", "${v.vmName}", "${x.message}", "${c.vmName}", "${g.title}", "${n}"} {
		if strings.Contains(string(js), bad) {
			t.Errorf("app.js interpolates %s without esc()", bad)
		}
	}
}

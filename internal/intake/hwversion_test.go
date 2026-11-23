// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package intake

import "testing"

func TestHWVersion(t *testing.T) {
	cases := map[string]int{"vmx-19": 19, "19": 19, "v13": 13, " VMX-7 ": 7, "": 0, "abc": 0, "-3": 0}
	for in, want := range cases {
		if got := hwVersion(in); got != want {
			t.Errorf("hwVersion(%q)=%d want %d", in, got, want)
		}
	}
}

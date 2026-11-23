// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: LicenseRef-Zyvor-Production-2.0

package model

import "strings"

func isHex(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// CanonicalMAC returns the lower-case colon form of a MAC address, or "" when
// the text is not six octets. It accepts aa:bb:.., aa-bb-.., aabb.ccdd.eeff
// and aabbccddeeff. Placeholder addresses are returned as they are; use
// NormalizeMAC to drop them.
func CanonicalMAC(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || len(s) > 32 {
		return ""
	}
	var hex string
	if strings.ContainsAny(s, ":-") {
		parts := strings.FieldsFunc(s, func(r rune) bool { return r == ':' || r == '-' })
		if len(parts) != 6 {
			return ""
		}
		for _, p := range parts {
			if len(p) != 2 {
				return ""
			}
			hex += p
		}
	} else {
		hex = strings.ReplaceAll(s, ".", "")
	}
	if len(hex) != 12 || !isHex(hex) {
		return ""
	}
	var b strings.Builder
	for i := 0; i < 12; i += 2 {
		if i > 0 {
			b.WriteByte(':')
		}
		b.WriteString(hex[i : i+2])
	}
	return b.String()
}

// IsPlaceholderMAC reports the all-zero and broadcast addresses, which are
// placeholders and not identities.
func IsPlaceholderMAC(canonical string) bool {
	return canonical == "00:00:00:00:00:00" || canonical == "ff:ff:ff:ff:ff:ff"
}

// NormalizeMAC is CanonicalMAC that also returns "" for placeholder addresses.
func NormalizeMAC(s string) string {
	c := CanonicalMAC(s)
	if IsPlaceholderMAC(c) {
		return ""
	}
	return c
}

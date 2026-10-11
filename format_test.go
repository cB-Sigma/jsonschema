// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package jsonschema

import "testing"

// The time and date-time formats check ranges, offsets and leap seconds;
// ipv6 refuses a zone.
func TestFormatRanges(t *testing.T) {
	for _, c := range []struct {
		format, value string
		ok            bool
	}{
		{"time", "08:30:06Z", true},
		{"time", "23:59:60Z", true},
		{"time", "15:59:60-08:00", true},
		{"time", "23:20:50.52+05:30", true},
		{"time", "24:00:00Z", false},
		{"time", "08:60:00Z", false},
		{"time", "08:30:61Z", false},
		{"time", "22:59:60Z", false},
		{"time", "23:59:60+01:00", false},
		{"time", "08:30:06+24:00", false},
		{"time", "08:30:06+05:60", false},
		{"date-time", "1998-12-31T23:59:60Z", true},
		{"date-time", "1998-12-31T15:59:60.123-08:00", true},
		{"date-time", "2026-02-29T10:00:00Z", false},
		{"date-time", "2026-10-11T10:00:00+25:00", false},
		{"date-time", "2026-10-11t10:00:00z", true},
		{"ipv6", "fe80::1", true},
		{"ipv6", "fe80::1%eth0", false},
	} {
		got := checkers[c.format](c.value) == ""
		if got != c.ok {
			t.Errorf("%s %q: valid %v, want %v", c.format, c.value, got, c.ok)
		}
	}
}

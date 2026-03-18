package exif

import (
	"testing"
	"time"
)

func TestStringMatch(t *testing.T) {
	testCases := []struct {
		name     string
		existing any
		expected string
		want     bool
	}{
		{name: "exact match", existing: "hello", expected: "hello", want: true},
		{name: "match with surrounding spaces", existing: "  hello  ", expected: "hello", want: true},
		{name: "nil returns false", existing: nil, expected: "hello", want: false},
		{name: "wrong type returns false", existing: 42, expected: "hello", want: false},
		{name: "mismatch", existing: "world", expected: "hello", want: false},
		{name: "empty strings match", existing: "", expected: "", want: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := StringMatch(tc.existing, tc.expected)
			if got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestDateTimeStringMatch(t *testing.T) {
	testCases := []struct {
		name     string
		existing any
		expected string
		want     bool
	}{
		{
			name:     "same moment with different format",
			existing: "2018:10:10 13:27:11.112+00:00",
			expected: "2018-10-10T13:27:11.112+00:00",
			want:     true,
		},
		{
			name:     "different moment",
			existing: "2018:10:10 13:27:11.000+00:00",
			expected: "2018-10-10T13:27:12.000+00:00",
			want:     false,
		},
		{
			name:     "unparseable returns false",
			existing: "not-a-date",
			expected: "2018-10-10T13:27:11.112+00:00",
			want:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := DateTimeStringMatch(tc.existing, tc.expected)
			if got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestIntMatch(t *testing.T) {
	testCases := []struct {
		name     string
		existing any
		expected int
		want     bool
	}{
		{name: "match", existing: float64(5), expected: 5, want: true},
		{name: "mismatch", existing: float64(3), expected: 5, want: false},
		{name: "nil returns false", existing: nil, expected: 5, want: false},
		{name: "wrong type returns false", existing: "5", expected: 5, want: false},
		{name: "zero match", existing: float64(0), expected: 0, want: true},
		{name: "negative match", existing: float64(-3), expected: -3, want: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := IntMatch(tc.existing, tc.expected)
			if got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestFloatMatchNilAndWrongType(t *testing.T) {
	if FloatMatch(nil, 1.0) {
		t.Fatal("nil should not match")
	}
	if FloatMatch("1.0", 1.0) {
		t.Fatal("string should not match")
	}
}

func TestAllLocationTagValuesMatch(t *testing.T) {
	strict := []string{"IPTC:City", "XMP-photoshop:City"}
	fallback := []string{"City"}

	testCases := []struct {
		name     string
		existing ExifTagMap
		expected string
		want     bool
	}{
		{
			name:     "all strict keys match",
			existing: ExifTagMap{"IPTC:City": "Paris", "XMP-photoshop:City": "Paris"},
			expected: "Paris",
			want:     true,
		},
		{
			name:     "one strict key mismatch",
			existing: ExifTagMap{"IPTC:City": "Paris", "XMP-photoshop:City": "Lyon"},
			expected: "Paris",
			want:     false,
		},
		{
			name:     "strict key present but nil value for second",
			existing: ExifTagMap{"IPTC:City": "Paris"},
			expected: "Paris",
			want:     false,
		},
		{
			name:     "fallback key match when no strict keys present",
			existing: ExifTagMap{"City": "Paris"},
			expected: "Paris",
			want:     true,
		},
		{
			name:     "fallback key mismatch",
			existing: ExifTagMap{"City": "Lyon"},
			expected: "Paris",
			want:     false,
		},
		{
			name:     "no keys present at all",
			existing: ExifTagMap{},
			expected: "Paris",
			want:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := AllLocationTagValuesMatch(tc.existing, strict, fallback, tc.expected)
			if got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestParseDateTime(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		wantErr bool
		check   func(t *testing.T, parsed time.Time)
	}{
		{
			name:  "RFC3339",
			input: "2025-12-10T16:56:36+01:00",
			check: func(t *testing.T, parsed time.Time) {
				if parsed.Hour() != 16 || parsed.Minute() != 56 {
					t.Fatalf("unexpected time: %v", parsed)
				}
			},
		},
		{
			name:  "RFC3339 with fractional seconds",
			input: "2025-12-10T16:56:36.123456789+01:00",
			check: func(t *testing.T, parsed time.Time) {
				if parsed.Nanosecond() == 0 {
					t.Fatalf("expected nanoseconds to be preserved")
				}
			},
		},
		{
			name:  "EXIF format",
			input: "2025:12:10 16:56:36",
			check: func(t *testing.T, parsed time.Time) {
				if parsed.Year() != 2025 || parsed.Month() != 12 || parsed.Day() != 10 {
					t.Fatalf("unexpected date: %v", parsed)
				}
			},
		},
		{
			name:  "EXIF format with offset",
			input: "2025:12:10 16:56:36+01:00",
			check: func(t *testing.T, parsed time.Time) {
				_, offset := parsed.Zone()
				if offset != 3600 {
					t.Fatalf("expected 3600 offset, got %d", offset)
				}
			},
		},
		{
			name:  "EXIF format with fractional seconds and offset",
			input: "2025:12:10 16:56:36.123+01:00",
			check: func(t *testing.T, parsed time.Time) {
				if parsed.Nanosecond() != 123000000 {
					t.Fatalf("expected 123ms, got %d", parsed.Nanosecond())
				}
				_, offset := parsed.Zone()
				if offset != 3600 {
					t.Fatalf("expected 3600 offset, got %d", offset)
				}
			},
		},
		{
			name:    "invalid format",
			input:   "not-a-date",
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := ParseDateTime(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.check != nil {
				tc.check(t, parsed)
			}
		})
	}
}

func TestBuildOffsetValues(t *testing.T) {
	testCases := []struct {
		name             string
		offsetSeconds    int
		wantStr          string
		wantTZ           int
		wantHasWholeHour bool
	}{
		{name: "UTC", offsetSeconds: 0, wantStr: "+00:00", wantTZ: 0, wantHasWholeHour: true},
		{name: "+1 hour", offsetSeconds: 3600, wantStr: "+01:00", wantTZ: 1, wantHasWholeHour: true},
		{name: "-5 hours", offsetSeconds: -5 * 3600, wantStr: "-05:00", wantTZ: -5, wantHasWholeHour: true},
		{name: "+5:30 (India)", offsetSeconds: 5*3600 + 30*60, wantStr: "+05:30", wantTZ: 0, wantHasWholeHour: false},
		{name: "-9:30", offsetSeconds: -(9*3600 + 30*60), wantStr: "-09:30", wantTZ: 0, wantHasWholeHour: false},
		{name: "+12 hours", offsetSeconds: 12 * 3600, wantStr: "+12:00", wantTZ: 12, wantHasWholeHour: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			str, tz, hasWholeHour := BuildOffsetValues(tc.offsetSeconds)
			if str != tc.wantStr {
				t.Fatalf("offset string: expected %q, got %q", tc.wantStr, str)
			}
			if tz != tc.wantTZ {
				t.Fatalf("tz hours: expected %d, got %d", tc.wantTZ, tz)
			}
			if hasWholeHour != tc.wantHasWholeHour {
				t.Fatalf("hasWholeHour: expected %v, got %v", tc.wantHasWholeHour, hasWholeHour)
			}
		})
	}
}

func TestDateTimeMatch(t *testing.T) {
	testCases := []struct {
		name     string
		val      any
		offset   any
		tzOffset any
		expected string
		want     bool
	}{
		{
			name:     "nil existing returns false",
			val:      nil,
			expected: "2025-12-10T16:56:36+01:00",
			want:     false,
		},
		{
			name:     "exact match with offset",
			val:      "2025:12:10 16:56:36",
			offset:   "+01:00",
			expected: "2025-12-10T16:56:36+01:00",
			want:     true,
		},
		{
			name:     "same moment different timezone",
			val:      "2025:12:10 17:56:36",
			offset:   "+02:00",
			expected: "2025-12-10T16:56:36+01:00",
			want:     true,
		},
		{
			name:     "different moment returns false",
			val:      "2025:12:10 18:00:00",
			offset:   "+01:00",
			expected: "2025-12-10T16:56:36+01:00",
			want:     false,
		},
		{
			name:     "timezone offset as float64",
			val:      "2025:12:10 16:56:36",
			tzOffset: float64(1),
			expected: "2025-12-10T16:56:36+01:00",
			want:     true,
		},
		{
			name:     "wrong type for existing returns false",
			val:      42,
			expected: "2025-12-10T16:56:36+01:00",
			want:     false,
		},
		{
			name:     "unparseable expected falls back to string compare",
			val:      "custom-date",
			expected: "custom-date",
			want:     true,
		},
		{
			name:     "unparseable expected string mismatch",
			val:      "custom-date",
			expected: "other-date",
			want:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := DateTimeMatch(tc.val, tc.offset, tc.tzOffset, tc.expected)
			if got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestOffsetSecondsArePlausibleExtraCases(t *testing.T) {
	testCases := []struct {
		name    string
		seconds int
		want    bool
	}{
		{name: "UTC", seconds: 0, want: true},
		{name: "+5:45 Nepal", seconds: 5*3600 + 45*60, want: true},
		{name: "+9:15", seconds: 9*3600 + 15*60, want: true},
		{name: "+14 hours max", seconds: 14 * 3600, want: true},
		{name: "-12 hours min", seconds: -12 * 3600, want: true},
		{name: "too positive", seconds: 15 * 3600, want: false},
		{name: "too negative", seconds: -13 * 3600, want: false},
		{name: "non-standard minutes +3:07", seconds: 3*3600 + 7*60, want: false},
		{name: "negative non-standard", seconds: -(3*3600 + 7*60), want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := OffsetSecondsArePlausible(tc.seconds)
			if got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestDateTimeMatchUnparseableExistingWithOffset(t *testing.T) {
	got := DateTimeMatch("not-a-date", "+01:00", nil, "2025-12-10T16:56:36+01:00")
	if got {
		t.Fatal("expected false when existing is not parseable even with offset appended")
	}
}

func TestDateTimeMatchExistingParsesAsUTCWhenNoOffset(t *testing.T) {
	got := DateTimeMatch("2025:12:10 16:56:36", nil, nil, "2025-12-10T16:56:36Z")
	if !got {
		t.Fatal("expected true when both parse as UTC and match")
	}
	got2 := DateTimeMatch("2025:12:10 17:00:00", nil, nil, "2025-12-10T16:56:36Z")
	if got2 {
		t.Fatal("expected false when times differ")
	}
}

func TestDeriveOffsetValuesForMissingOffsetEdgeCases(t *testing.T) {
	t.Run("unparseable expected returns false", func(t *testing.T) {
		existing := ExifTagMap{"DateTimeOriginal": "2025:12:10 17:56:36"}
		_, _, _, canInfer := DeriveOffsetValuesForMissingOffset(existing, "not-a-date")
		if canInfer {
			t.Fatal("expected canInfer=false for unparseable expected")
		}
	})

	t.Run("nil DateTimeOriginal returns false", func(t *testing.T) {
		existing := ExifTagMap{}
		_, _, _, canInfer := DeriveOffsetValuesForMissingOffset(existing, "2025-12-10T16:56:36Z")
		if canInfer {
			t.Fatal("expected canInfer=false when DateTimeOriginal is nil")
		}
	})

	t.Run("unparseable existing DateTimeOriginal returns false", func(t *testing.T) {
		existing := ExifTagMap{"DateTimeOriginal": "garbage"}
		_, _, _, canInfer := DeriveOffsetValuesForMissingOffset(existing, "2025-12-10T16:56:36Z")
		if canInfer {
			t.Fatal("expected canInfer=false for unparseable existing date")
		}
	})

	t.Run("half hour offset", func(t *testing.T) {
		existing := ExifTagMap{"DateTimeOriginal": "2025:12:10 22:26:36"}
		offsetStr, _, hasWholeHour, canInfer := DeriveOffsetValuesForMissingOffset(existing, "2025-12-10T16:56:36Z")
		if !canInfer {
			t.Fatal("expected canInfer=true for half-hour offset")
		}
		if offsetStr != "+05:30" {
			t.Fatalf("expected +05:30, got %s", offsetStr)
		}
		if hasWholeHour {
			t.Fatal("expected hasWholeHour=false for 5:30 offset")
		}
	})
}

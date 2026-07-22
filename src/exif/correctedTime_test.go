package exif

import (
	"testing"

	"github.com/majorfi/immich-exif/model"
)

func strPtr(s string) *string { return &s }

func TestCorrectedLocalTimeReanchorsToIANAZone(t *testing.T) {
	info := &model.ExifInfo{
		DateTimeOriginal: strPtr("2004-06-05T12:25:32.000Z"),
		TimeZone:         strPtr("Europe/Paris"),
	}
	got, ok := CorrectedLocalTime(info)
	if !ok {
		t.Fatal("expected ok=true")
	}
	// Paris is UTC+2 in June (CEST): 12:25 UTC -> 14:25 local.
	if h := got.Hour(); h != 14 {
		t.Fatalf("expected wall-clock hour 14, got %d", h)
	}
}

func TestCorrectedLocalTimeReanchorsToFixedOffset(t *testing.T) {
	info := &model.ExifInfo{
		DateTimeOriginal: strPtr("2004-06-05T12:25:32.000Z"),
		TimeZone:         strPtr("UTC+05:30"),
	}
	got, ok := CorrectedLocalTime(info)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if h, m := got.Hour(), got.Minute(); h != 17 || m != 55 {
		t.Fatalf("expected 17:55, got %02d:%02d", h, m)
	}
}

func TestCorrectedLocalTimeNilInfo(t *testing.T) {
	if _, ok := CorrectedLocalTime(nil); ok {
		t.Fatal("expected ok=false for nil info")
	}
}

func TestCorrectedLocalTimeMissingDate(t *testing.T) {
	if _, ok := CorrectedLocalTime(&model.ExifInfo{}); ok {
		t.Fatal("expected ok=false when DateTimeOriginal is absent")
	}
	if _, ok := CorrectedLocalTime(&model.ExifInfo{DateTimeOriginal: strPtr("")}); ok {
		t.Fatal("expected ok=false when DateTimeOriginal is empty")
	}
}

func TestCorrectedLocalTimeUnparsableDate(t *testing.T) {
	if _, ok := CorrectedLocalTime(&model.ExifInfo{DateTimeOriginal: strPtr("not-a-date")}); ok {
		t.Fatal("expected ok=false for an unparsable date")
	}
}

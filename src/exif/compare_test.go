package exif

import (
	"strings"
	"testing"

	"github.com/majorfi/immich-exif/model"
)

func TestCompareMetadataNilExif(t *testing.T) {
	changes := CompareMetadata(nil, ExifTagMap{})
	if len(changes) != 0 {
		t.Fatalf("expected no changes for nil exif, got %d", len(changes))
	}
}

func TestCompareGPSNilCoordinates(t *testing.T) {
	exif := &model.ExifInfo{}
	changes := CompareMetadata(exif, ExifTagMap{})
	args := CollectExifArgs(changes)
	if len(args) != 0 {
		t.Fatalf("expected no GPS args for nil coordinates, got %v", args)
	}
}

func TestCompareGPSMatch(t *testing.T) {
	lat := 48.856613
	lon := 2.352222
	exif := &model.ExifInfo{Latitude: &lat, Longitude: &lon}
	existing := ExifTagMap{
		"GPSLatitude":           lat,
		"GPSLongitude":          lon,
		"XMP-exif:GPSLatitude":  lat,
		"XMP-exif:GPSLongitude": lon,
	}
	changes := CompareMetadata(exif, existing)
	args := CollectExifArgs(changes)
	if len(args) != 0 {
		t.Fatalf("expected no GPS args when matching, got %v", args)
	}
}

func TestCompareGPSBackfillsXMPWhenEXIFMatches(t *testing.T) {
	lat := 48.856613
	lon := 2.352222
	exif := &model.ExifInfo{Latitude: &lat, Longitude: &lon}
	existing := ExifTagMap{
		"GPSLatitude":  lat,
		"GPSLongitude": lon,
	}
	changes := CompareMetadata(exif, existing)
	args := CollectExifArgs(changes)
	hasXMPLat := false
	hasXMPLon := false
	for _, arg := range args {
		if strings.HasPrefix(arg, "-XMP-exif:GPSLatitude=") {
			hasXMPLat = true
		}
		if strings.HasPrefix(arg, "-XMP-exif:GPSLongitude=") {
			hasXMPLon = true
		}
	}
	if !hasXMPLat || !hasXMPLon {
		t.Fatalf("expected XMP GPS backfill when EXIF matches but XMP missing, got %v", args)
	}
}

func TestCompareGPSBackfillProducesDiffs(t *testing.T) {
	lat := 48.856613
	lon := 2.352222
	exif := &model.ExifInfo{Latitude: &lat, Longitude: &lon}
	existing := ExifTagMap{
		"GPSLatitude":  lat,
		"GPSLongitude": lon,
	}
	changes := CompareMetadata(exif, existing)
	entries := CollectDiffEntries(changes)
	if len(entries) == 0 {
		t.Fatal("expected non-empty diffs for GPS XMP backfill to trigger user confirmation")
	}
}

func TestCompareGPSMismatch(t *testing.T) {
	lat := 48.856613
	lon := 2.352222
	exif := &model.ExifInfo{Latitude: &lat, Longitude: &lon}
	existing := ExifTagMap{
		"GPSLatitude":  49.0,
		"GPSLongitude": 3.0,
	}
	changes := CompareMetadata(exif, existing)
	args := CollectExifArgs(changes)
	if len(args) == 0 {
		t.Fatal("expected GPS args for mismatch")
	}
}

func TestCompareGPSNegativeCoordinates(t *testing.T) {
	lat := -33.8688
	lon := -151.2093
	exif := &model.ExifInfo{Latitude: &lat, Longitude: &lon}
	changes := CompareMetadata(exif, ExifTagMap{})
	args := CollectExifArgs(changes)

	hasLatRef := false
	hasLonRef := false
	for _, arg := range args {
		if arg == "-GPSLatitudeRef=S" {
			hasLatRef = true
		}
		if arg == "-GPSLongitudeRef=W" {
			hasLonRef = true
		}
	}
	if !hasLatRef {
		t.Fatal("expected GPSLatitudeRef=S for negative latitude")
	}
	if !hasLonRef {
		t.Fatal("expected GPSLongitudeRef=W for negative longitude")
	}
}

func TestCompareDescriptionEmpty(t *testing.T) {
	desc := ""
	exif := &model.ExifInfo{Description: &desc}
	changes := CompareMetadata(exif, ExifTagMap{})
	if len(CollectExifArgs(changes)) != 0 {
		t.Fatal("expected no args for empty description")
	}
}

func TestCompareDescriptionNil(t *testing.T) {
	exif := &model.ExifInfo{}
	changes := CompareMetadata(exif, ExifTagMap{})
	if len(CollectExifArgs(changes)) != 0 {
		t.Fatal("expected no args for nil description")
	}
}

func TestCompareRatingNil(t *testing.T) {
	exif := &model.ExifInfo{}
	changes := CompareMetadata(exif, ExifTagMap{})
	if len(CollectExifArgs(changes)) != 0 {
		t.Fatal("expected no args for nil rating")
	}
}

func TestCompareRatingZeroSkipped(t *testing.T) {
	rating := 0
	exif := &model.ExifInfo{Rating: &rating}
	changes := CompareMetadata(exif, ExifTagMap{})
	if len(CollectExifArgs(changes)) != 0 {
		t.Fatal("expected no args for zero rating")
	}
}

func TestCompareLocationNilValue(t *testing.T) {
	exif := &model.ExifInfo{}
	changes := CompareMetadata(exif, ExifTagMap{})
	if len(CollectExifArgs(changes)) != 0 {
		t.Fatal("expected no args for nil location fields")
	}
}

func TestCompareLocationEmptyValue(t *testing.T) {
	empty := ""
	exif := &model.ExifInfo{City: &empty, State: &empty, Country: &empty}
	changes := CompareMetadata(exif, ExifTagMap{})
	if len(CollectExifArgs(changes)) != 0 {
		t.Fatal("expected no args for empty location fields")
	}
}

func TestCompareDateTimeNil(t *testing.T) {
	exif := &model.ExifInfo{}
	changes := CompareMetadata(exif, ExifTagMap{})
	if len(CollectExifArgs(changes)) != 0 {
		t.Fatal("expected no args for nil datetime")
	}
}

func TestCompareDateTimeEmpty(t *testing.T) {
	empty := ""
	exif := &model.ExifInfo{DateTimeOriginal: &empty}
	changes := CompareMetadata(exif, ExifTagMap{})
	if len(CollectExifArgs(changes)) != 0 {
		t.Fatal("expected no args for empty datetime")
	}
}

func TestCompareSimpleStringNil(t *testing.T) {
	exif := &model.ExifInfo{}
	changes := CompareMetadata(exif, ExifTagMap{})
	if len(CollectExifArgs(changes)) != 0 {
		t.Fatal("expected no args for nil simple strings")
	}
}

func TestCompareSimpleStringMatch(t *testing.T) {
	make_ := "Canon"
	model_ := "EOS R5"
	lens := "RF 50mm"
	exif := &model.ExifInfo{Make: &make_, Model: &model_, LensModel: &lens}
	existing := ExifTagMap{
		"Make":      "Canon",
		"Model":     "EOS R5",
		"LensModel": "RF 50mm",
	}
	changes := CompareMetadata(exif, existing)
	if len(CollectExifArgs(changes)) != 0 {
		t.Fatal("expected no args when simple strings match")
	}
}

func TestCompareSimpleStringMismatchSkipsWhenExisting(t *testing.T) {
	make_ := "Canon"
	exif := &model.ExifInfo{Make: &make_}
	existing := ExifTagMap{
		"Make": "Nikon",
	}
	changes := CompareMetadata(exif, existing)
	args := CollectExifArgs(changes)
	if len(args) != 0 {
		t.Fatalf("expected no args when existing Make differs (only-if-empty), got %v", args)
	}
}

func TestCollectExifArgsEmpty(t *testing.T) {
	args := CollectExifArgs(nil)
	if len(args) != 0 {
		t.Fatalf("expected empty, got %v", args)
	}
}

func TestCollectDiffEntriesEmpty(t *testing.T) {
	entries := CollectDiffEntries(nil)
	if len(entries) != 0 {
		t.Fatalf("expected empty, got %v", entries)
	}
}

func TestAppendDiffAdd(t *testing.T) {
	diffs := appendDiff(nil, "Tag", nil, "new-value", func(a any, s string) bool { return false })
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	if diffs[0].Symbol != model.DiffAdd {
		t.Fatalf("expected DiffAdd, got %s", string(diffs[0].Symbol))
	}
	if diffs[0].Old != "(none)" {
		t.Fatalf("expected (none), got %s", diffs[0].Old)
	}
}

func TestAppendDiffChange(t *testing.T) {
	diffs := appendDiff(nil, "Tag", "old", "new", func(a any, s string) bool { return false })
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	if diffs[0].Symbol != model.DiffChange {
		t.Fatalf("expected DiffChange, got %s", string(diffs[0].Symbol))
	}
}

func TestAppendDiffMatch(t *testing.T) {
	diffs := appendDiff(nil, "Tag", "same", "same", func(a any, s string) bool { return true })
	if len(diffs) != 0 {
		t.Fatalf("expected 0 diffs for match, got %d", len(diffs))
	}
}

func TestCompareDateTimeUnparseableExpectedMatchesString(t *testing.T) {
	dt := "custom-format-date"
	exif := &model.ExifInfo{DateTimeOriginal: &dt}
	existing := ExifTagMap{
		"DateTimeOriginal":          "custom-format-date",
		"XMP-exif:DateTimeOriginal": "custom-format-date",
		"XMP-xmp:CreateDate":        "custom-format-date",
	}
	changes := CompareMetadata(exif, existing)
	if len(CollectExifArgs(changes)) != 0 {
		t.Fatal("expected no args when unparseable datetime matches by string including XMP")
	}
}

func TestCompareDateTimeUnparseableBackfillsXMP(t *testing.T) {
	dt := "custom-format-date"
	exif := &model.ExifInfo{DateTimeOriginal: &dt}
	existing := ExifTagMap{
		"DateTimeOriginal": "custom-format-date",
	}
	changes := CompareMetadata(exif, existing)
	args := CollectExifArgs(changes)
	hasXMPDate := false
	hasCreateDate := false
	for _, arg := range args {
		if arg == "-XMP-exif:DateTimeOriginal=custom-format-date" {
			hasXMPDate = true
		}
		if arg == "-XMP-xmp:CreateDate=custom-format-date" {
			hasCreateDate = true
		}
	}
	if !hasXMPDate || !hasCreateDate {
		t.Fatalf("expected XMP datetime backfill when EXIF matches but XMP missing, got %v", args)
	}
}

func TestCompareDateTimeUnparseableExpectedMismatch(t *testing.T) {
	dt := "custom-format-date"
	exif := &model.ExifInfo{DateTimeOriginal: &dt}
	existing := ExifTagMap{
		"DateTimeOriginal": "different-date",
	}
	changes := CompareMetadata(exif, existing)
	if len(CollectExifArgs(changes)) == 0 {
		t.Fatal("expected args when unparseable datetime mismatches")
	}
}

func TestCompareDateTimeXMPSameMomentDifferentFormat(t *testing.T) {
	dt := "2018-10-10T13:27:11.112+00:00"
	exif := &model.ExifInfo{DateTimeOriginal: &dt}
	existing := ExifTagMap{
		"DateTimeOriginal":          "2018:10:10 13:27:11",
		"OffsetTimeOriginal":        "+00:00",
		"TimeZoneOffset":            float64(0),
		"XMP-exif:DateTimeOriginal": "2018:10:10 13:27:11.112+00:00",
		"XMP-xmp:CreateDate":        "2018:10:10 13:27:11.112+00:00",
	}
	changes := CompareMetadata(exif, existing)
	args := CollectExifArgs(changes)
	if len(args) != 0 {
		t.Fatalf("expected no args when XMP datetime differs only by format, got %v", args)
	}
}

func TestCompareDateTimeMissingOffsetInfersOffset(t *testing.T) {
	dt := "2025-12-10T16:56:36Z"
	exif := &model.ExifInfo{DateTimeOriginal: &dt}
	existing := ExifTagMap{
		"DateTimeOriginal": "2025:12:10 17:56:36",
	}
	changes := CompareMetadata(exif, existing)
	args := CollectExifArgs(changes)
	hasOffset := false
	hasTZ := false
	for _, arg := range args {
		if arg == "-OffsetTimeOriginal=+01:00" {
			hasOffset = true
		}
		if arg == "-TimeZoneOffset=1" {
			hasTZ = true
		}
	}
	if !hasOffset {
		t.Fatalf("expected OffsetTimeOriginal arg, got %v", args)
	}
	if !hasTZ {
		t.Fatalf("expected TimeZoneOffset arg, got %v", args)
	}
}

func TestCompareDateTimeMissingOffsetAlreadyCorrect(t *testing.T) {
	dt := "2025-12-10T16:56:36Z"
	exif := &model.ExifInfo{DateTimeOriginal: &dt}
	existing := ExifTagMap{
		"DateTimeOriginal":          "2025:12:10 17:56:36",
		"OffsetTimeOriginal":        "+01:00",
		"TimeZoneOffset":            float64(1),
		"XMP-exif:DateTimeOriginal": "2025-12-10T16:56:36Z",
		"XMP-xmp:CreateDate":        "2025-12-10T16:56:36Z",
	}
	changes := CompareMetadata(exif, existing)
	args := CollectExifArgs(changes)
	if len(args) != 0 {
		t.Fatalf("expected no args when offset already correct, got %v", args)
	}
}

func TestCompareDateTimeMissingOffsetHalfHour(t *testing.T) {
	dt := "2025-12-10T16:56:36Z"
	exif := &model.ExifInfo{DateTimeOriginal: &dt}
	existing := ExifTagMap{
		"DateTimeOriginal": "2025:12:10 22:26:36",
	}
	changes := CompareMetadata(exif, existing)
	args := CollectExifArgs(changes)
	hasOffset := false
	for _, arg := range args {
		if arg == "-OffsetTimeOriginal=+05:30" {
			hasOffset = true
		}
	}
	if !hasOffset {
		t.Fatalf("expected OffsetTimeOriginal=+05:30, got %v", args)
	}
	for _, arg := range args {
		if arg == "-TimeZoneOffset=0" {
			t.Fatal("should not set TimeZoneOffset for non-whole-hour offset")
		}
	}
}

func TestCompareLocationFallbackDiffEntries(t *testing.T) {
	city := "Paris"
	exif := &model.ExifInfo{City: &city}
	existing := ExifTagMap{
		"City": "Lyon",
	}
	changes := CompareMetadata(exif, existing)
	entries := CollectDiffEntries(changes)
	if len(entries) == 0 {
		t.Fatal("expected diff entries for location fallback mismatch")
	}
	if entries[0].Symbol != model.DiffChange {
		t.Fatalf("expected DiffChange symbol, got %s", string(entries[0].Symbol))
	}
}

func TestCompareLocationNoExistingTags(t *testing.T) {
	city := "Paris"
	exif := &model.ExifInfo{City: &city}
	changes := CompareMetadata(exif, ExifTagMap{})
	entries := CollectDiffEntries(changes)
	if len(entries) == 0 {
		t.Fatal("expected diff entries when no location tags exist")
	}
	if entries[0].Symbol != model.DiffAdd {
		t.Fatalf("expected DiffAdd symbol, got %s", string(entries[0].Symbol))
	}
}

func TestCompareDateTimeFullMomentMatchOffsetMatchTZMismatch(t *testing.T) {
	dt := "2025-12-10T16:56:36+01:00"
	exif := &model.ExifInfo{DateTimeOriginal: &dt}
	existing := ExifTagMap{
		"DateTimeOriginal":   "2025:12:10 16:56:36",
		"OffsetTimeOriginal": "+01:00",
		"TimeZoneOffset":     float64(5),
	}
	changes := CompareMetadata(exif, existing)
	args := CollectExifArgs(changes)
	hasTZ := false
	for _, arg := range args {
		if arg == "-TimeZoneOffset=1" {
			hasTZ = true
		}
	}
	if !hasTZ {
		t.Fatalf("expected TimeZoneOffset=1 arg, got %v", args)
	}
}

func TestCompareDateTimeFullNoExisting(t *testing.T) {
	dt := "2025-12-10T16:56:36+01:00"
	exif := &model.ExifInfo{DateTimeOriginal: &dt}
	changes := CompareMetadata(exif, ExifTagMap{})
	args := CollectExifArgs(changes)
	if len(args) == 0 {
		t.Fatal("expected args when no existing datetime")
	}
	hasDateArg := false
	hasOffsetArg := false
	for _, arg := range args {
		if arg == "-DateTimeOriginal=2025:12:10 16:56:36" {
			hasDateArg = true
		}
		if arg == "-OffsetTimeOriginal=+01:00" {
			hasOffsetArg = true
		}
	}
	if !hasDateArg {
		t.Fatalf("expected DateTimeOriginal arg, got %v", args)
	}
	if !hasOffsetArg {
		t.Fatalf("expected OffsetTimeOriginal arg, got %v", args)
	}
}

func TestCompareDateTimeFullMomentMatchMissingOffsetTimeOriginal(t *testing.T) {
	dt := "2025-12-10T16:56:36+01:00"
	exif := &model.ExifInfo{DateTimeOriginal: &dt}
	existing := ExifTagMap{
		"DateTimeOriginal": "2025:12:10 16:56:36",
		"TimeZoneOffset":   float64(1),
	}
	changes := CompareMetadata(exif, existing)
	args := CollectExifArgs(changes)
	hasOffsetArg := false
	for _, arg := range args {
		if arg == "-OffsetTimeOriginal=+01:00" {
			hasOffsetArg = true
		}
	}
	if !hasOffsetArg {
		t.Fatalf("expected OffsetTimeOriginal arg when OffsetTimeOriginal is missing, got %v", args)
	}
}

func TestCompareDateTimeMissingOffsetImplausibleFallsBack(t *testing.T) {
	dt := "2025-12-10T16:56:36Z"
	exif := &model.ExifInfo{DateTimeOriginal: &dt}
	existing := ExifTagMap{
		"DateTimeOriginal": "2025-12-11 12:56:36",
	}
	changes := CompareMetadata(exif, existing)
	args := CollectExifArgs(changes)
	if len(args) == 0 {
		t.Fatal("expected args for implausible offset fallback")
	}
}

func TestCompareRatingMismatch(t *testing.T) {
	rating := 4
	exif := &model.ExifInfo{Rating: &rating}
	existing := ExifTagMap{
		"Rating":        float64(3),
		"RatingPercent": float64(60),
	}
	changes := CompareMetadata(exif, existing)
	args := CollectExifArgs(changes)
	if len(args) == 0 {
		t.Fatal("expected args for rating mismatch")
	}
	hasRating := false
	for _, arg := range args {
		if arg == "-Rating=4" {
			hasRating = true
		}
	}
	if !hasRating {
		t.Fatalf("expected -Rating=4 arg, got %v", args)
	}
}

func TestCompareRatingMatch(t *testing.T) {
	rating := 3
	exif := &model.ExifInfo{Rating: &rating}
	existing := ExifTagMap{
		"Rating":         float64(3),
		"RatingPercent":  float64(60),
		"XMP-xmp:Rating": float64(3),
	}
	changes := CompareMetadata(exif, existing)
	args := CollectExifArgs(changes)
	if len(args) != 0 {
		t.Fatalf("expected no args for matching rating, got %v", args)
	}
}

func TestCompareDescriptionMismatch(t *testing.T) {
	desc := "New caption"
	exif := &model.ExifInfo{Description: &desc}
	existing := ExifTagMap{
		"ImageDescription": "Old caption",
	}
	changes := CompareMetadata(exif, existing)
	args := CollectExifArgs(changes)
	if len(args) == 0 {
		t.Fatal("expected args for description mismatch")
	}
}

func TestCompareDescriptionMatch(t *testing.T) {
	desc := "Same"
	exif := &model.ExifInfo{Description: &desc}
	existing := ExifTagMap{
		"ImageDescription":      "Same",
		"XPComment":             "Same",
		"XMP-dc:Description":    "Same",
		"IPTC:Caption-Abstract": "Same",
	}
	changes := CompareMetadata(exif, existing)
	args := CollectExifArgs(changes)
	if len(args) != 0 {
		t.Fatalf("expected no args for matching description, got %v", args)
	}
}

func TestCompareDateTimeFullMomentMatchNilOffsetAddsTZOffset(t *testing.T) {
	dt := "2025-12-10T15:56:36Z"
	exif := &model.ExifInfo{DateTimeOriginal: &dt}
	existing := ExifTagMap{
		"DateTimeOriginal": "2025:12:10 16:56:36",
		"TimeZoneOffset":   float64(1),
	}
	changes := CompareMetadata(exif, existing)
	args := CollectExifArgs(changes)
	hasOffset := false
	hasTZ := false
	for _, arg := range args {
		if arg == "-OffsetTimeOriginal=+00:00" {
			hasOffset = true
		}
		if arg == "-TimeZoneOffset=0" {
			hasTZ = true
		}
	}
	if !hasOffset {
		t.Fatalf("expected OffsetTimeOriginal arg, got %v", args)
	}
	if !hasTZ {
		t.Fatalf("expected TimeZoneOffset arg, got %v", args)
	}
}

func TestCompareLocationWritesExplicitXMPNamespace(t *testing.T) {
	city := "Paris"
	exif := &model.ExifInfo{City: &city}
	changes := CompareMetadata(exif, ExifTagMap{})
	args := CollectExifArgs(changes)
	hasExplicit := false
	for _, arg := range args {
		if arg == "-XMP-photoshop:City=Paris" {
			hasExplicit = true
		}
		if arg == "-XMP:City=Paris" {
			t.Fatal("should use explicit XMP-photoshop namespace, not ambiguous XMP")
		}
	}
	if !hasExplicit {
		t.Fatalf("expected -XMP-photoshop:City=Paris, got %v", args)
	}
}

func TestCompareDescriptionWritesXMPAndIPTC(t *testing.T) {
	desc := "A beautiful sunset"
	exif := &model.ExifInfo{Description: &desc}
	changes := CompareMetadata(exif, ExifTagMap{})
	args := CollectExifArgs(changes)
	expected := map[string]bool{
		"-ImageDescription=A beautiful sunset":      false,
		"-XPComment=A beautiful sunset":             false,
		"-XMP-dc:Description=A beautiful sunset":    false,
		"-IPTC:Caption-Abstract=A beautiful sunset": false,
	}
	for _, arg := range args {
		if _, ok := expected[arg]; ok {
			expected[arg] = true
		}
	}
	for arg, found := range expected {
		if !found {
			t.Fatalf("missing arg %s, got %v", arg, args)
		}
	}
}

func TestCompareDescriptionMatchIncludesXMPAndIPTC(t *testing.T) {
	desc := "Same"
	exif := &model.ExifInfo{Description: &desc}
	existing := ExifTagMap{
		"ImageDescription":      "Same",
		"XPComment":             "Same",
		"XMP-dc:Description":    "Same",
		"IPTC:Caption-Abstract": "Same",
	}
	changes := CompareMetadata(exif, existing)
	args := CollectExifArgs(changes)
	if len(args) != 0 {
		t.Fatalf("expected no args when all description fields match, got %v", args)
	}
}

func TestCompareRatingWritesXMP(t *testing.T) {
	rating := 4
	exif := &model.ExifInfo{Rating: &rating}
	changes := CompareMetadata(exif, ExifTagMap{})
	args := CollectExifArgs(changes)
	hasXMP := false
	for _, arg := range args {
		if arg == "-XMP-xmp:Rating=4" {
			hasXMP = true
		}
	}
	if !hasXMP {
		t.Fatalf("expected -XMP-xmp:Rating=4, got %v", args)
	}
}

func TestCompareRatingMatchIncludesXMP(t *testing.T) {
	rating := 3
	exif := &model.ExifInfo{Rating: &rating}
	existing := ExifTagMap{
		"Rating":         float64(3),
		"RatingPercent":  float64(60),
		"XMP-xmp:Rating": float64(3),
	}
	changes := CompareMetadata(exif, existing)
	args := CollectExifArgs(changes)
	if len(args) != 0 {
		t.Fatalf("expected no args when all rating fields match, got %v", args)
	}
}

func TestCompareGPSWritesXMP(t *testing.T) {
	lat := 48.856613
	lon := 2.352222
	exif := &model.ExifInfo{Latitude: &lat, Longitude: &lon}
	changes := CompareMetadata(exif, ExifTagMap{})
	args := CollectExifArgs(changes)
	hasXMPLat := false
	hasXMPLon := false
	for _, arg := range args {
		if strings.HasPrefix(arg, "-XMP-exif:GPSLatitude=") {
			hasXMPLat = true
		}
		if strings.HasPrefix(arg, "-XMP-exif:GPSLongitude=") {
			hasXMPLon = true
		}
	}
	if !hasXMPLat {
		t.Fatalf("expected XMP-exif:GPSLatitude arg, got %v", args)
	}
	if !hasXMPLon {
		t.Fatalf("expected XMP-exif:GPSLongitude arg, got %v", args)
	}
}

func TestCompareDateTimeWritesXMP(t *testing.T) {
	dt := "2025-12-10T16:56:36+01:00"
	exif := &model.ExifInfo{DateTimeOriginal: &dt}
	changes := CompareMetadata(exif, ExifTagMap{})
	args := CollectExifArgs(changes)
	hasXMPDate := false
	hasCreateDate := false
	for _, arg := range args {
		if arg == "-XMP-exif:DateTimeOriginal=2025-12-10T16:56:36+01:00" {
			hasXMPDate = true
		}
		if arg == "-XMP-xmp:CreateDate=2025-12-10T16:56:36+01:00" {
			hasCreateDate = true
		}
	}
	if !hasXMPDate {
		t.Fatalf("expected XMP-exif:DateTimeOriginal arg, got %v", args)
	}
	if !hasCreateDate {
		t.Fatalf("expected XMP-xmp:CreateDate arg, got %v", args)
	}
}

func TestCompareSimpleStringOnlyIfEmptySkipsExisting(t *testing.T) {
	make_ := "Canon"
	exif := &model.ExifInfo{Make: &make_}
	existing := ExifTagMap{
		"Make": "Nikon",
	}
	changes := CompareMetadata(exif, existing)
	args := CollectExifArgs(changes)
	for _, arg := range args {
		if strings.Contains(arg, "Make") {
			t.Fatalf("should not overwrite existing Make, got %v", args)
		}
	}
}

func TestCompareSimpleStringOnlyIfEmptyWritesWhenMissing(t *testing.T) {
	make_ := "Canon"
	exif := &model.ExifInfo{Make: &make_}
	changes := CompareMetadata(exif, ExifTagMap{})
	args := CollectExifArgs(changes)
	hasMake := false
	for _, arg := range args {
		if arg == "-Make=Canon" {
			hasMake = true
		}
	}
	if !hasMake {
		t.Fatalf("expected -Make=Canon when file has no Make, got %v", args)
	}
}

func TestCompareSimpleStringOnlyIfEmptyWritesWhenBlank(t *testing.T) {
	make_ := "Canon"
	exif := &model.ExifInfo{Make: &make_}
	existing := ExifTagMap{
		"Make": "",
	}
	changes := CompareMetadata(exif, existing)
	args := CollectExifArgs(changes)
	hasMake := false
	for _, arg := range args {
		if arg == "-Make=Canon" {
			hasMake = true
		}
	}
	if !hasMake {
		t.Fatalf("expected -Make=Canon when existing Make is blank, got %v", args)
	}
}

func TestCompareSimpleStringOnlyIfEmptyWritesWhenWhitespace(t *testing.T) {
	make_ := "Canon"
	exif := &model.ExifInfo{Make: &make_}
	existing := ExifTagMap{
		"Make": "  ",
	}
	changes := CompareMetadata(exif, existing)
	args := CollectExifArgs(changes)
	hasMake := false
	for _, arg := range args {
		if arg == "-Make=Canon" {
			hasMake = true
		}
	}
	if !hasMake {
		t.Fatalf("expected -Make=Canon when existing Make is whitespace, got %v", args)
	}
}

func TestCompareLocationStrictKeysMismatch(t *testing.T) {
	city := "Paris"
	exif := &model.ExifInfo{City: &city}
	existing := ExifTagMap{
		"IPTC:City":          "Lyon",
		"XMP-photoshop:City": "Lyon",
	}
	changes := CompareMetadata(exif, existing)
	entries := CollectDiffEntries(changes)
	if len(entries) == 0 {
		t.Fatal("expected diff entries for strict key mismatch")
	}
	hasIPTC := false
	hasXMP := false
	for _, e := range entries {
		if strings.Contains(e.Tag, "IPTC") {
			hasIPTC = true
		}
		if strings.Contains(e.Tag, "XMP") {
			hasXMP = true
		}
	}
	if !hasIPTC || !hasXMP {
		t.Fatalf("expected both IPTC and XMP diff entries, got %v", entries)
	}
}

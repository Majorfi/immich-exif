package process

import (
	"image"
	"image/color"
	"testing"

	"github.com/majorfi/immich-exif/model"
)

func patternImage(width, height int, contrast float64) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			value := float64(x*255/width+y*96/height) / 351 * 255
			if x < width/3 && y < height/2 {
				value = 255
			}
			level := uint8(128 + (value-128)*contrast)
			img.Set(x, y, color.RGBA{R: level, G: uint8(255 * x / width), B: uint8(255 * y / height), A: 255})
		}
	}
	return img
}

func transformed(src *image.RGBA, width, height int, at func(x, y int) (int, int)) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			sx, sy := at(x, y)
			out.Set(x, y, src.At(sx, sy))
		}
	}
	return out
}

func editsOf(raw ...string) []model.AssetEdit {
	edits := make([]model.AssetEdit, 0, len(raw)/2)
	for i := 0; i < len(raw); i += 2 {
		edits = append(edits, model.AssetEdit{Action: raw[i], Parameters: []byte(raw[i+1])})
	}
	return edits
}

func TestRenderMatchesEditsQuarterTurnAndMirror(t *testing.T) {
	preview := patternImage(120, 80, 1)
	rotated := transformed(preview, 80, 120, func(x, y int) (int, int) { return y, 79 - x })
	if ok, d := renderMatchesEdits(preview, rotated, editsOf("rotate", `{"angle":90}`), 1200, 800); !ok {
		t.Fatalf("a clockwise quarter turn must match, difference %.1f", d)
	}
	if ok, _ := renderMatchesEdits(preview, rotated, editsOf("rotate", `{"angle":270}`), 1200, 800); ok {
		t.Fatal("a render turned the other way must not match")
	}
	mirrored := transformed(preview, 120, 80, func(x, y int) (int, int) { return 119 - x, y })
	if ok, _ := renderMatchesEdits(preview, mirrored, editsOf("mirror", `{"axis":"horizontal"}`), 1200, 800); !ok {
		t.Fatal("a left-right mirror must match")
	}
	if ok, _ := renderMatchesEdits(preview, mirrored, editsOf("mirror", `{"axis":"vertical"}`), 1200, 800); ok {
		t.Fatal("a stale mirror on the other axis must not match")
	}
}

func TestRenderMatchesEditsCrop(t *testing.T) {
	preview := patternImage(100, 80, 1)
	cropped := transformed(preview, 50, 40, func(x, y int) (int, int) { return 20 + x, 10 + y })
	crop := editsOf("crop", `{"x":200,"y":100,"width":500,"height":400}`)
	if ok, d := renderMatchesEdits(preview, cropped, crop, 1000, 800); !ok {
		t.Fatalf("the cropped region must match, difference %.1f", d)
	}
	moved := editsOf("crop", `{"x":0,"y":400,"width":500,"height":400}`)
	if ok, _ := renderMatchesEdits(preview, cropped, moved, 1000, 800); ok {
		t.Fatal("a render of another crop window must not match")
	}
}

func TestRenderMatchesEditsCatchesStaleMirrorOnLowContrast(t *testing.T) {
	preview := patternImage(120, 80, 0.05)
	for x := range 120 {
		for y := range 80 {
			c := preview.RGBAAt(x, y)
			preview.SetRGBA(x, y, color.RGBA{R: c.R, G: 128, B: 128, A: 255})
		}
	}
	stale := transformed(preview, 120, 80, func(x, y int) (int, int) { return 119 - x, y })
	ok, d := renderMatchesEdits(preview, stale, nil, 1200, 800)
	if d >= renderMismatchThreshold {
		t.Fatalf("fixture must stay under the absolute threshold to exercise the orientation check, difference %.1f", d)
	}
	if ok {
		t.Fatalf("a stale mirror must not pass just because the photo is flat, difference %.1f", d)
	}
}

func TestRenderMatchesEditsAcceptsSymmetricPhoto(t *testing.T) {
	preview := image.NewRGBA(image.Rect(0, 0, 60, 40))
	for x := range 60 {
		for y := range 40 {
			preview.Set(x, y, color.RGBA{R: 90, G: 140, B: 200, A: 255})
		}
	}
	if ok, _ := renderMatchesEdits(preview, preview, editsOf("mirror", `{"axis":"horizontal"}`), 600, 400); !ok {
		t.Fatal("a uniform photo looks the same in every orientation and must pass")
	}
}

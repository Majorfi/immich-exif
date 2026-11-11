package process

import (
	"image"
	_ "image/jpeg"
	"math"

	_ "golang.org/x/image/webp"

	"github.com/majorfi/immich-exif/model"
)

const renderCheckGrid = 24
const renderCheckSamples = 8

const renderMismatchThreshold = 12.0

type point struct{ x, y float64 }

type editStep struct {
	action string
	params model.EditParameters
}

func editSteps(edits []model.AssetEdit) []editStep {
	steps := make([]editStep, 0, len(edits))
	for _, edit := range edits {
		steps = append(steps, editStep{action: edit.Action, params: edit.Params()})
	}
	return steps
}

var orientationVariants = [][]model.AssetEdit{
	{{Action: "mirror", Parameters: []byte(`{"axis":"horizontal"}`)}},
	{{Action: "mirror", Parameters: []byte(`{"axis":"vertical"}`)}},
	{{Action: "rotate", Parameters: []byte(`{"angle":90}`)}},
	{{Action: "rotate", Parameters: []byte(`{"angle":180}`)}},
	{{Action: "rotate", Parameters: []byte(`{"angle":270}`)}},
	{{Action: "rotate", Parameters: []byte(`{"angle":90}`)}, {Action: "mirror", Parameters: []byte(`{"axis":"horizontal"}`)}},
	{{Action: "rotate", Parameters: []byte(`{"angle":90}`)}, {Action: "mirror", Parameters: []byte(`{"axis":"vertical"}`)}},
}

// renderMatchesEdits reports whether a render shows the edits, and the mean
// difference measured. A render is rejected when it is far from the expected
// image, or when one of the other seven orientations of it fits clearly
// better: that catches a stale mirror or turn even on low-contrast photos,
// while a symmetric photo, where every orientation fits alike, still passes.
func renderMatchesEdits(preview, render image.Image, edits []model.AssetEdit, width, height int) (bool, float64) {
	difference := renderDifference(preview, render, edits, width, height)
	if difference > renderMismatchThreshold {
		return false, difference
	}
	for _, variant := range orientationVariants {
		other := renderDifference(preview, render, append(append([]model.AssetEdit{}, edits...), variant...), width, height)
		if other < difference/2 && difference-other > 1 {
			return false, difference
		}
	}
	return true, difference
}

// renderDifference compares a render with what the edits should produce from
// the unedited preview, both reduced to a coarse grid, and returns the mean
// per-channel difference (0-255). Each render cell is mapped back through the
// edits in reverse (mirrors and clockwise quarter turns undone, then the crop)
// onto the preview, so a render left over from other edits of the same size
// (a mirror, a half turn, a moved crop) does not match. width and height are
// the original's size after EXIF orientation, the space crops are given in.
func renderDifference(preview, render image.Image, edits []model.AssetEdit, width, height int) float64 {
	steps := editSteps(edits)
	total := 0.0
	for row := range renderCheckGrid {
		for col := range renderCheckGrid {
			var want, got [3]float64
			for sy := range renderCheckSamples {
				for sx := range renderCheckSamples {
					edited := point{
						x: (float64(col) + (float64(sx)+0.5)/renderCheckSamples) / renderCheckGrid,
						y: (float64(row) + (float64(sy)+0.5)/renderCheckSamples) / renderCheckGrid,
					}
					addColor(&got, render, edited)
					addColor(&want, preview, sourcePoint(edited, steps, width, height))
				}
			}
			for channel := range 3 {
				total += math.Abs(got[channel]-want[channel]) / (renderCheckSamples * renderCheckSamples)
			}
		}
	}
	return total / (renderCheckGrid * renderCheckGrid * 3)
}

func sourcePoint(p point, steps []editStep, width, height int) point {
	for i := len(steps) - 1; i >= 0; i-- {
		step := steps[i]
		switch {
		case step.action == "rotate" && step.params.Angle == 90:
			p = point{x: p.y, y: 1 - p.x}
		case step.action == "rotate" && step.params.Angle == 180:
			p = point{x: 1 - p.x, y: 1 - p.y}
		case step.action == "rotate" && step.params.Angle == 270:
			p = point{x: 1 - p.y, y: p.x}
		case step.action == "mirror" && step.params.Axis == "horizontal":
			p.x = 1 - p.x
		case step.action == "mirror" && step.params.Axis == "vertical":
			p.y = 1 - p.y
		}
	}
	for _, step := range steps {
		if step.action == "crop" && width > 0 && height > 0 {
			params := step.params
			p = point{
				x: (float64(params.X) + p.x*float64(params.Width)) / float64(width),
				y: (float64(params.Y) + p.y*float64(params.Height)) / float64(height),
			}
		}
	}
	return p
}

func addColor(sum *[3]float64, img image.Image, p point) {
	bounds := img.Bounds()
	x := bounds.Min.X + min(int(p.x*float64(bounds.Dx())), bounds.Dx()-1)
	y := bounds.Min.Y + min(int(p.y*float64(bounds.Dy())), bounds.Dy()-1)
	r, g, b, _ := img.At(x, y).RGBA()
	sum[0] += float64(r >> 8)
	sum[1] += float64(g >> 8)
	sum[2] += float64(b >> 8)
}

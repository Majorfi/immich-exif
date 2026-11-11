package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
)

type AssetEdit struct {
	Action     string          `json:"action"`
	Parameters json.RawMessage `json:"parameters"`
}

type EditParameters struct {
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Angle  int    `json:"angle"`
	Axis   string `json:"axis"`
}

type AssetEdits struct {
	Edits []AssetEdit `json:"edits"`
}

var BakeableEditActions = []string{"crop", "rotate", "mirror"}

func (e AssetEdit) Params() EditParameters {
	var params EditParameters
	_ = json.Unmarshal(e.Parameters, &params)
	return params
}

func SameEdits(a, b []AssetEdit) bool {
	return slices.EqualFunc(a, b, func(x, y AssetEdit) bool {
		return x.Action == y.Action && sameJSON(x.Parameters, y.Parameters)
	})
}

func sameJSON(a, b json.RawMessage) bool {
	var x, y map[string]json.RawMessage
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	return maps.EqualFunc(x, y, func(v, w json.RawMessage) bool {
		var compactV, compactW bytes.Buffer
		return json.Compact(&compactV, v) == nil && json.Compact(&compactW, w) == nil && bytes.Equal(compactV.Bytes(), compactW.Bytes())
	})
}

func EditedSize(edits []AssetEdit, width, height int) (int, int) {
	for _, edit := range edits {
		if edit.Action == "crop" {
			params := edit.Params()
			width, height = params.Width, params.Height
		}
	}
	for _, edit := range edits {
		angle := edit.Params().Angle
		if edit.Action == "rotate" && (angle == 90 || angle == 270) {
			width, height = height, width
		}
	}
	return width, height
}

func DescribeEdits(edits []AssetEdit) string {
	parts := make([]string, 0, len(edits))
	for _, edit := range edits {
		params := edit.Params()
		switch edit.Action {
		case "crop":
			parts = append(parts, fmt.Sprintf("crop %dx%d", params.Width, params.Height))
		case "rotate":
			parts = append(parts, fmt.Sprintf("rotate %d°", params.Angle))
		case "mirror":
			parts = append(parts, "mirror "+params.Axis)
		default:
			parts = append(parts, edit.Action)
		}
	}
	return strings.Join(parts, ", ")
}

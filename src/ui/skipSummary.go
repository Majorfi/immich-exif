package ui

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/majorfi/immich-exif/model"
)

const skippedIDsShown = 5

func skippedSummaryLines(results []model.ProcessResult) []string {
	groups := map[string][]string{}
	for _, r := range results {
		if r.Status != model.StatusSkipped || r.DuplicateID != "" {
			continue
		}
		reason := r.Message
		if reason == "" {
			reason = "no reason given"
		}
		groups[reason] = append(groups[reason], model.ShortID(r.AssetID))
	}

	reasons := slices.Collect(maps.Keys(groups))
	slices.SortFunc(reasons, func(a, b string) int {
		return cmp.Or(cmp.Compare(len(groups[b]), len(groups[a])), cmp.Compare(a, b))
	})
	lines := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		ids := groups[reason]
		shown := strings.Join(ids[:min(len(ids), skippedIDsShown)], ", ")
		if len(ids) > skippedIDsShown {
			shown += fmt.Sprintf(" +%d more", len(ids)-skippedIDsShown)
		}
		lines = append(lines, fmt.Sprintf("%d × %s (%s)", len(ids), model.SanitizeForTerminal(reason), shown))
	}
	return lines
}

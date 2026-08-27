package main

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/model"
)

// repairStacks re-points stacks that lost their primary asset. Immich shows
// only the primary of a stack in the timeline, and a trashed asset keeps the
// primary slot until Immich purges it. An older version of this tool trashed
// the primary without moving the slot. This hides the full stack, including
// the assets the tool did not touch.
func repairStacks(client *api.ImmichClient, cfg *model.Config) int {
	stacks, err := client.ListStacks()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing stacks: %v\n", err)
		return 1
	}

	scan := scanStacks(client, stacks)
	for _, skipped := range scan.unreadable {
		fmt.Fprintf(os.Stderr, "Stack %s: cannot read its primary %s, skipping: %v\n",
			model.ShortID(skipped.stackID), model.ShortID(skipped.primaryID), skipped.err)
	}
	for _, stackID := range scan.emptied {
		fmt.Printf("Stack %s has no live member left; leaving it alone.\n", model.ShortID(stackID))
	}
	if len(scan.broken) == 0 {
		fmt.Printf("Checked %d stack(s): nothing to repair.\n", len(stacks))
		return 0
	}

	fmt.Printf("\n%d stack(s) point at a trashed primary and are hidden from the timeline:\n", len(scan.broken))
	for _, repair := range scan.broken {
		fmt.Printf("  stack %s: primary %s (trashed) -> %s (%s)\n",
			model.ShortID(repair.stackID), model.ShortID(repair.trashedPrimaryID),
			model.ShortID(repair.newPrimaryID), model.SanitizeForTerminal(repair.newPrimaryName))
	}

	if cfg.DryRun {
		fmt.Println("\nDry run: no stack was modified.")
		return 0
	}
	if !cfg.Yes && !confirmRepairStacks(os.Stdin, os.Stdout) {
		fmt.Println("Aborted.")
		return 0
	}

	failed := 0
	for _, repair := range scan.broken {
		if err := client.UpdateStackPrimary(repair.stackID, repair.newPrimaryID); err != nil {
			fmt.Fprintf(os.Stderr, "Error repairing stack %s: %v\n", model.ShortID(repair.stackID), err)
			failed++
			continue
		}
		fmt.Printf("Repaired stack %s\n", model.ShortID(repair.stackID))
	}
	if failed > 0 {
		fmt.Fprintf(os.Stderr, "\n%d of %d stack(s) could not be repaired.\n", failed, len(scan.broken))
		return 1
	}
	fmt.Printf("\nRepaired %d stack(s).\n", len(scan.broken))
	return 0
}

type stackRepair struct {
	stackID          string
	trashedPrimaryID string
	newPrimaryID     string
	newPrimaryName   string
}

type unreadablePrimary struct {
	stackID   string
	primaryID string
	err       error
}

type stackScan struct {
	broken     []stackRepair
	emptied    []string
	unreadable []unreadablePrimary
}

// scanStacks sorts stacks into those needing a new primary, those with no live
// member left to promote, and those whose primary cannot be read. A single
// unreadable primary must not sink the whole scan: an asset with `locked`
// visibility is invisible to an API key (Immich excludes it from owner access
// unless the session holds an elevated permission), so aborting there would
// make the mode fail permanently on any library holding one.
func scanStacks(client *api.ImmichClient, stacks []model.StackResponse) stackScan {
	var scan stackScan

	for _, stack := range stacks {
		// The server already filters this list to live members with timeline or
		// archive visibility, so a primary missing from it is trashed, hidden or
		// locked.
		if containsAssetID(stack.Assets, stack.PrimaryAssetID) {
			continue
		}
		primary, err := client.GetAsset(stack.PrimaryAssetID)
		if err != nil {
			scan.unreadable = append(scan.unreadable, unreadablePrimary{
				stackID:   stack.ID,
				primaryID: stack.PrimaryAssetID,
				err:       err,
			})
			continue
		}
		// A hidden primary is absent from the list too, and its stack is meant
		// to stay out of the timeline; promoting over it would surface what the
		// owner deliberately hid.
		if !primary.IsTrashed {
			continue
		}
		if len(stack.Assets) == 0 {
			scan.emptied = append(scan.emptied, stack.ID)
			continue
		}
		// Assets come ordered by fileCreatedAt ascending, so the first one is
		// what Immich itself promotes when it purges a trashed primary.
		scan.broken = append(scan.broken, stackRepair{
			stackID:          stack.ID,
			trashedPrimaryID: stack.PrimaryAssetID,
			newPrimaryID:     stack.Assets[0].ID,
			newPrimaryName:   stack.Assets[0].OriginalFileName,
		})
	}
	return scan
}

func containsAssetID(assets []model.AssetResponse, assetID string) bool {
	for _, asset := range assets {
		if asset.ID == assetID {
			return true
		}
	}
	return false
}

func confirmRepairStacks(reader io.Reader, writer io.Writer) bool {
	fmt.Fprint(writer, "\nRepair these stacks? [y/N]: ")
	input, err := bufio.NewReader(reader).ReadString('\n')
	if err != nil && len(input) == 0 {
		return false
	}
	return isAffirmativeInput(input)
}

package process

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/model"
)

var refreshWaitAttempts = 30

const refreshRepeatEvery = 10

type inPlaceTarget struct {
	root  string
	local string
	info  os.FileInfo
	sum   []byte
}

// resolveInPlace returns the local file an external asset is updated through,
// or a reason to skip it. An empty path without a reason means no mapping
// covers the asset. Only a path-hashed checksum is safe: Immich never
// recomputes a content checksum, so rewriting such a file would leave it stale.
func resolveInPlace(cfg *model.Config, asset model.AssetResponse) (inPlaceTarget, string) {
	local, root, ok := cfg.MapServerPath(asset.OriginalPath)
	if !ok {
		return inPlaceTarget{}, ""
	}
	if !model.IsPathChecksum(asset) {
		return inPlaceTarget{}, "Immich hashes this asset's content, so updating it in place would leave its checksum stale"
	}
	if note := checkLocalFile(root, local); note != "" {
		return inPlaceTarget{}, note
	}
	return inPlaceTarget{root: root, local: local}, ""
}

// checkLocalFile refuses what an atomic rename would not carry over or could be
// redirected through: a symlink anywhere under the mapped root, a file with
// other hard links, or one owned by another user.
func checkLocalFile(root, local string) string {
	relative, err := filepath.Rel(root, local)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Sprintf("%s is outside the mapped directory", local)
	}
	current := root
	var info os.FileInfo
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err = os.Lstat(current)
		if err != nil {
			return fmt.Sprintf("cannot read %s: %v", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Sprintf("%s is a symlink; in-place updates refuse symlinks", current)
		}
	}
	if !info.Mode().IsRegular() {
		return fmt.Sprintf("%s is not a regular file", local)
	}
	if hasACL(local) {
		return fmt.Sprintf("%s carries an access control list the tool cannot carry over", local)
	}
	links, uid, ok := fileOwnership(info)
	if !ok {
		return "cannot read the file's ownership on this system"
	}
	if links > 1 {
		return fmt.Sprintf("%s has %d hard links; rewriting it would split them", local, links)
	}
	if uid != os.Getuid() {
		return fmt.Sprintf("%s belongs to another user; rewriting it would change its owner", local)
	}
	return ""
}

// prepareInPlace proves the mapped file is the asset Immich serves, byte for
// byte, so a wrong mapping can never rewrite an unrelated file, and records
// its state for the pre-commit race check.
func prepareInPlace(client *api.ImmichClient, cfg *model.Config, asset model.AssetResponse, target inPlaceTarget, downloaded string) (inPlaceTarget, string, error) {
	served, err := fileChecksumSHA1(downloaded)
	if err != nil {
		return target, "", err
	}
	target.sum, err = fileChecksumSHA1(target.local)
	if err != nil {
		return target, "", err
	}
	if !bytes.Equal(served, target.sum) {
		return target, fmt.Sprintf("%s differs from the file Immich serves; check -in-place-map", target.local), nil
	}
	target.info, err = os.Lstat(target.local)
	if err != nil {
		return target, "", err
	}
	note, err := sidecarNote(client, cfg, asset, target.local)
	return target, note, err
}

// commitInPlace swaps the rewritten copy over the library file once nothing
// changed since it was read, then asks Immich to re-read it. Nothing after the
// rename is retried: the file is already updated at that point. A write by
// another program between the last check and the rename is not detected.
func commitInPlace(client *api.ImmichClient, cfg *model.Config, asset *model.AssetResponse, rewritten string, target inPlaceTarget, emitter model.EventEmitter, cancelled func() bool) model.ProcessResult {
	result := func(status model.ResultStatus, message string) model.ProcessResult {
		if status == model.StatusFailed {
			message = fmt.Sprintf("[%s] %s", model.ShortID(asset.ID), message)
		}
		return model.ProcessResult{AssetID: asset.ID, Status: status, Message: message}
	}
	note, err := sidecarNote(client, cfg, *asset, target.local)
	if err != nil {
		return result(model.StatusFailed, fmt.Sprintf("re-check sidecars: %v", err))
	}
	if note == "" {
		note = checkLocalFile(target.root, target.local)
	}
	if note == "" {
		note = changedSince(target)
	}
	if note != "" {
		return result(model.StatusSkipped, note)
	}

	emitter.EmitProgress(model.ProgressEvent{AssetID: asset.ID, Filename: asset.OriginalFileName, Step: "Updating " + target.local + " in place..."})
	if err := replaceFile(target.local, rewritten, target.info); errors.Is(err, errNotDurable) {
		return result(model.StatusFailed, fmt.Sprintf("file updated in place, but %v", err))
	} else if err != nil {
		return result(model.StatusFailed, fmt.Sprintf("write in place: %v (the library file was not changed)", err))
	}
	reread, err := waitForRefresh(client, *asset, cancelled)
	if err != nil {
		return result(model.StatusFailed, fmt.Sprintf("file updated in place, but %v", err))
	}
	if !reread {
		return result(model.StatusSuccess, "updated in place; Immich re-reads it at its next library scan")
	}
	return result(model.StatusSuccess, "updated in place")
}

func changedSince(target inPlaceTarget) string {
	info, err := os.Lstat(target.local)
	if err != nil {
		return fmt.Sprintf("cannot re-read %s: %v", target.local, err)
	}
	sum, err := fileChecksumSHA1(target.local)
	if err != nil || !os.SameFile(info, target.info) || info.Size() != target.info.Size() || !info.ModTime().Equal(target.info.ModTime()) || !bytes.Equal(sum, target.sum) {
		return fmt.Sprintf("%s changed on disk while processing; re-run to pick it up", target.local)
	}
	return ""
}

// waitForRefresh asks Immich to re-read the file until it reports the new
// modification time. A network or Docker file share can still serve Immich the
// old attributes for a few seconds, and a refresh run in that window reads the
// old file, so the request is repeated rather than trusted once. Giving up is
// not a failure: the file's new mtime makes the next library scan re-read it.
func waitForRefresh(client *api.ImmichClient, asset model.AssetResponse, cancelled func() bool) (bool, error) {
	for attempt := 0; attempt < refreshWaitAttempts; attempt++ {
		if cancelled != nil && cancelled() {
			return false, nil
		}
		if attempt%refreshRepeatEvery == 0 {
			if err := client.RefreshMetadata(asset.ID); err != nil {
				hint := ""
				if isPermissionDenied(err) {
					hint = " (the API key needs job.create and asset.update)"
				}
				return false, fmt.Errorf("asking Immich to re-read it failed%s: %w; run Refresh metadata on the asset", hint, err)
			}
		}
		sleepFn(time.Second)
		fresh, err := client.GetAsset(asset.ID)
		if err != nil {
			return false, fmt.Errorf("checking that Immich re-read it failed: %w", err)
		}
		if !fresh.FileModifiedAt.Equal(asset.FileModifiedAt) {
			return true, nil
		}
	}
	return false, nil
}

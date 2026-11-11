package process

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/exif"
	"github.com/majorfi/immich-exif/model"
)

var refreshWaitAttempts = 60

const refreshRepeatEvery = 10

type inPlaceTarget struct {
	root  string
	local string
	info  os.FileInfo
	sum   []byte
}

func mapServerPath(cfg *model.Config, serverPath string) (string, string, bool) {
	serverPath = path.Clean(serverPath)
	var best model.PathMapping
	found := false
	for _, mapping := range cfg.InPlaceMaps {
		prefix := strings.TrimSuffix(mapping.Server, "/") + "/"
		if (serverPath == mapping.Server || strings.HasPrefix(serverPath, prefix)) && len(mapping.Server) > len(best.Server) {
			best, found = mapping, true
		}
	}
	if !found {
		return "", "", false
	}
	relative := strings.TrimPrefix(strings.TrimPrefix(serverPath, best.Server), "/")
	return filepath.Join(best.Local, filepath.FromSlash(relative)), best.Local, true
}

// resolveInPlace returns the local file an external asset is updated through,
// or a reason to skip it. An empty path without a reason means no mapping
// covers the asset. Only a path-hashed checksum is safe: Immich never
// recomputes a content checksum, so rewriting such a file would leave it stale.
func resolveInPlace(cfg *model.Config, asset model.AssetResponse) (inPlaceTarget, string) {
	local, root, ok := mapServerPath(cfg, asset.OriginalPath)
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
	served, err := fileSHA1(downloaded)
	if err != nil {
		return target, "", err
	}
	target.sum, err = fileSHA1(target.local)
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

// sidecarNote reports a sidecar Immich would apply over the rewritten file
// with a value that disagrees with its database. Immich writes its own
// sidecar next to edited external files; that one agrees and is harmless.
// City, state and country are left out: Immich derives them from GPS.
func sidecarNote(client *api.ImmichClient, cfg *model.Config, asset model.AssetResponse, local string) (string, error) {
	registered, err := client.GetSidecarPaths(asset.ID)
	if code, ok := statusCodeOf(err); ok && code == http.StatusNotFound {
		return "listing an asset's sidecars needs Immich 3.2 or later", nil
	}
	if isPermissionDenied(err) {
		return "", fmt.Errorf("listing sidecars was denied, -in-place-map needs the API key's asset.file.read permission: %w", err)
	}
	if err != nil {
		return "", fmt.Errorf("list sidecars: %w", err)
	}
	candidates := []string{local + ".xmp", strings.TrimSuffix(local, filepath.Ext(local)) + ".xmp"}
	for _, serverPath := range registered {
		mapped, _, ok := mapServerPath(cfg, serverPath)
		if !ok {
			return fmt.Sprintf("its sidecar %s is outside the mapped paths", serverPath), nil
		}
		candidates = append(candidates, mapped)
	}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if seen[candidate] {
			continue
		}
		seen[candidate] = true
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return "", err
		}
		tags, err := exif.ReadExifTagsFn(candidate)
		if err != nil {
			return "", fmt.Errorf("read sidecar %s: %w", candidate, err)
		}
		if cfg.Faces && tags["XMP-mwg-rs:RegionInfo"] != nil {
			return fmt.Sprintf("sidecar %s holds face regions that Immich would apply over the file's", filepath.Base(candidate)), nil
		}
		for _, change := range exif.CompareAssetMetadata(asset, tags) {
			for _, diff := range change.Diffs {
				if diff.Symbol == model.DiffChange && !isLocationTag(diff.Tag) {
					return fmt.Sprintf("sidecar %s disagrees with Immich on %s (%s, Immich has %s) and Immich would apply it over the file", filepath.Base(candidate), diff.Tag, diff.Old, diff.New), nil
				}
			}
		}
	}
	return "", nil
}

func isLocationTag(tag string) bool {
	for _, word := range []string{"City", "State", "Province", "Country"} {
		if strings.Contains(tag, word) {
			return true
		}
	}
	return false
}

// commitInPlace swaps the rewritten copy over the library file once nothing
// changed since it was read, then asks Immich to re-read it. Nothing after the
// rename is retried: the file is already updated at that point.
func commitInPlace(client *api.ImmichClient, cfg *model.Config, asset *model.AssetResponse, rewritten string, target inPlaceTarget, emitter model.EventEmitter) model.ProcessResult {
	result := func(status model.ResultStatus, message string) model.ProcessResult {
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
	if err := replaceFile(target.local, rewritten, target.info.Mode().Perm()); err != nil {
		return result(model.StatusFailed, fmt.Sprintf("write in place: %v (the library file was not changed)", err))
	}
	if err := waitForRefresh(client, *asset); err != nil {
		return result(model.StatusFailed, fmt.Sprintf("file updated in place, but %v", err))
	}
	return result(model.StatusSuccess, "updated in place")
}

func changedSince(target inPlaceTarget) string {
	info, err := os.Lstat(target.local)
	if err != nil {
		return fmt.Sprintf("cannot re-read %s: %v", target.local, err)
	}
	if !os.SameFile(info, target.info) || info.Size() != target.info.Size() || !info.ModTime().Equal(target.info.ModTime()) {
		return fmt.Sprintf("%s changed on disk while processing; re-run to pick it up", target.local)
	}
	sum, err := fileSHA1(target.local)
	if err != nil || !bytes.Equal(sum, target.sum) {
		return fmt.Sprintf("%s changed on disk while processing; re-run to pick it up", target.local)
	}
	return ""
}

// waitForRefresh asks Immich to re-read the file until it reports the new
// modification time. A network or Docker file share can still serve Immich the
// old attributes for a few seconds, and a refresh run in that window reads the
// old file, so the request is repeated rather than trusted once.
func waitForRefresh(client *api.ImmichClient, asset model.AssetResponse) error {
	for attempt := 0; ; attempt++ {
		if attempt%refreshRepeatEvery == 0 {
			if err := client.RefreshMetadata(asset.ID); err != nil {
				hint := ""
				if isPermissionDenied(err) {
					hint = " (the API key needs job.create and asset.update)"
				}
				return fmt.Errorf("asking Immich to re-read it failed%s: %w; run Refresh metadata on the asset", hint, err)
			}
		}
		sleepFn(time.Second)
		fresh, err := client.GetAsset(asset.ID)
		if err != nil {
			return fmt.Errorf("checking that Immich re-read it failed: %w", err)
		}
		if !fresh.FileModifiedAt.Equal(asset.FileModifiedAt) {
			return nil
		}
		if attempt+1 >= refreshWaitAttempts {
			return fmt.Errorf("Immich has not re-read it after %d checks; it will on its next metadata refresh or library scan", attempt+1)
		}
	}
}

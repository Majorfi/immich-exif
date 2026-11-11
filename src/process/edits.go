package process

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/exif"
	"github.com/majorfi/immich-exif/model"
)

var dimensionsWaitAttempts = 120

var editSettleTime = time.Minute

type bakePlan struct {
	edits      []model.AssetEdit
	width      int
	height     int
	wantWidth  int
	wantHeight int
	skipNote   string
}

type bakedFile struct {
	path     string
	tags     exif.ExifTagMap
	edits    string
	skipNote string
}

// planBake runs the checks that need no download. Immich keeps serving the
// previous render while a new one is generated, and no API tells which edits a
// render belongs to; every edit and every finished render bumps updatedAt, so
// an asset changed in the last minute (on the server's clock) is skipped.
func planBake(client *api.ImmichClient, asset model.AssetResponse) (bakePlan, error) {
	now, err := client.ServerTime()
	if err != nil {
		return bakePlan{}, fmt.Errorf("read server time: %w", err)
	}
	if now.Sub(asset.UpdatedAt) < editSettleTime {
		return bakePlan{skipNote: "asset changed less than a minute ago; Immich may still be rendering its edits, re-run shortly"}, nil
	}
	edits, err := client.GetAssetEdits(asset.ID)
	if err != nil {
		return bakePlan{}, editsReadError(err)
	}
	if len(edits) == 0 {
		return bakePlan{skipNote: "Immich reports the asset as edited but returned no edits; re-run once its editor settles"}, nil
	}
	for _, edit := range edits {
		if !slices.Contains(model.BakeableEditActions, edit.Action) {
			return bakePlan{skipNote: fmt.Sprintf("Immich edit %q is not supported by -apply-edits yet", edit.Action)}, nil
		}
	}
	width, height, _, err := client.GetExifGeometry(asset.ID)
	if err != nil {
		return bakePlan{}, fmt.Errorf("fetch dimensions: %w", err)
	}
	if width == 0 || height == 0 {
		return bakePlan{skipNote: "Immich has not extracted this asset's dimensions; re-run once its metadata job finishes"}, nil
	}
	wantWidth, wantHeight := model.EditedSize(edits, width, height)
	return bakePlan{edits: edits, width: width, height: height, wantWidth: wantWidth, wantHeight: wantHeight}, nil
}

func bakeEdits(client *api.ImmichClient, asset model.AssetResponse, plan bakePlan, originalPath string, onProgress func(percent int)) (bakedFile, error) {
	renderDir, err := os.MkdirTemp(filepath.Dir(originalPath), "edited-*")
	if err != nil {
		return bakedFile{}, fmt.Errorf("create render dir: %w", err)
	}
	originalSum, err := fileSHA1(originalPath)
	if err != nil {
		return bakedFile{}, fmt.Errorf("hash original: %w", err)
	}
	renderPath, err := client.DownloadEditedAsset(asset.ID, renderDir, originalSum, onProgress)
	if errors.Is(err, api.ErrEditedRenderMissing) {
		return bakedFile{skipNote: "Immich has not rendered the edits yet; re-run once its editor job finishes"}, nil
	}
	if err != nil {
		return bakedFile{}, fmt.Errorf("download edited render: %w", err)
	}

	if err := exif.CopyMetadataFn(originalPath, renderPath); err != nil {
		return bakedFile{}, fmt.Errorf("copy metadata onto render: %w", err)
	}
	tags, err := exif.ReadExifTagsFn(renderPath)
	if err != nil {
		return bakedFile{}, fmt.Errorf("read render exif: %w", err)
	}
	gotWidth, gotHeight := intTag(tags, "ImageWidth"), intTag(tags, "ImageHeight")
	if gotWidth != plan.wantWidth || gotHeight != plan.wantHeight {
		return bakedFile{skipNote: fmt.Sprintf("edited render is %dx%d but the edits produce %dx%d; Immich may still be re-rendering, re-run later", gotWidth, gotHeight, plan.wantWidth, plan.wantHeight)}, nil
	}
	if skipNote, err := checkRenderContent(client, asset.ID, renderPath, plan); err != nil || skipNote != "" {
		return bakedFile{skipNote: skipNote}, err
	}
	return bakedFile{path: renderPath, tags: tags, edits: model.DescribeEdits(plan.edits)}, nil
}

func checkRenderContent(client *api.ImmichClient, assetID, renderPath string, plan bakePlan) (string, error) {
	previewData, err := client.GetUneditedPreview(assetID)
	if isPermissionDenied(err) {
		return "", fmt.Errorf("reading the preview was denied, -apply-edits needs the API key's asset.view permission to verify renders: %w", err)
	}
	if err != nil {
		return "", fmt.Errorf("download preview to verify the render: %w", err)
	}
	preview, _, err := image.Decode(bytes.NewReader(previewData))
	if err != nil {
		return fmt.Sprintf("cannot decode Immich's preview to verify the render (%v); use JPEG or WebP previews", err), nil
	}
	file, err := os.Open(renderPath)
	if err != nil {
		return "", fmt.Errorf("open render: %w", err)
	}
	defer file.Close()
	render, _, err := image.Decode(file)
	if err != nil {
		return fmt.Sprintf("cannot decode the edited render to verify it (%v); set Immich's full-size image format to JPEG or WebP", err), nil
	}
	if matches, difference := renderMatchesEdits(preview, render, plan.edits, plan.width, plan.height); !matches {
		return fmt.Sprintf("edited render does not match its edits (difference %.1f); Immich may still be re-rendering, re-run later", difference), nil
	}
	return "", nil
}

func bakedTagsChange(asset model.AssetResponse, existing exif.ExifTagMap) *exif.TagChange {
	values := tagValues(asset)
	if len(values) == 0 {
		return nil
	}
	current := tagListValues(existing)
	slices.Sort(current)
	if slices.Equal(values, current) {
		return nil
	}
	args := make([]string, 0, len(values))
	for _, value := range values {
		args = append(args, "-XMP-digiKam:TagsList="+value)
	}
	old := "(none)"
	symbol := model.DiffAdd
	if len(current) > 0 {
		old, symbol = strings.Join(current, ", "), model.DiffChange
	}
	return &exif.TagChange{
		Diffs: []model.DiffEntry{{Tag: "XMP-digiKam:TagsList", Symbol: symbol, Old: old, New: strings.Join(values, ", ")}},
		Args:  args,
	}
}

func tagValues(asset model.AssetResponse) []string {
	values := make([]string, 0, len(asset.Tags))
	for _, tag := range asset.Tags {
		values = append(values, tag.Value)
	}
	slices.Sort(values)
	return values
}

func tagListValues(existing exif.ExifTagMap) []string {
	raw, err := json.Marshal(existing["TagsList"])
	if err != nil {
		return nil
	}
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		return list
	}
	var single string
	if json.Unmarshal(raw, &single) == nil && single != "" {
		return []string{single}
	}
	return nil
}

type replacementPlan struct {
	edits       []model.AssetEdit
	orientation string
	copySidecar bool
}

// planReplacement reads, before anything is uploaded, what an edited asset's
// replacement needs. A baked file never gets the original's sidecar: Immich
// applies a sidecar over the file, and its orientation, dimensions or regions
// describe the pixels before the edits. The API cannot read a sidecar to clean
// it, so it is left with the trashed original. Edits to carry over are read
// with the orientation they are expressed in.
func (u *ModernUploader) planReplacement(asset *model.AssetResponse) (replacementPlan, error) {
	plan := replacementPlan{copySidecar: !(u.ApplyEdits && asset.IsEdited)}
	if !asset.IsEdited || u.ApplyEdits {
		return plan, nil
	}
	_, _, orientation, err := u.Client.GetExifGeometry(asset.ID)
	if err != nil {
		return plan, fmt.Errorf("read orientation: %w", err)
	}
	plan.orientation = uprightAsOne(orientation)
	edits, err := u.Client.GetAssetEdits(asset.ID)
	if isPermissionDenied(err) {
		return plan, nonRetryable(editsReadError(err))
	}
	if err != nil {
		return plan, editsReadError(err)
	}
	plan.edits = edits
	return plan, nil
}

func uprightAsOne(orientation string) string {
	if orientation == "" {
		return "1"
	}
	return orientation
}

func (u *ModernUploader) carryEdits(asset *model.AssetResponse, targetID string, plan replacementPlan, emitter model.EventEmitter) error {
	edits := plan.edits
	if len(edits) == 0 {
		return nil
	}
	notDeleted := fmt.Sprintf("target asset %s exists but old %s NOT deleted", targetID, asset.ID)
	orientation, err := u.waitForDimensions(targetID)
	if err != nil {
		return fmt.Errorf("%w (%s)", err, notDeleted)
	}
	if orientation != plan.orientation {
		return fmt.Errorf("target asset %s resolves orientation %s where the original resolved %s (a sidecar sets it), so its edits would land on other coordinates; refusing to carry them (%s)", model.ShortID(targetID), orientation, plan.orientation, notDeleted)
	}
	current, err := u.Client.GetAssetEdits(targetID)
	if err != nil {
		return fmt.Errorf("read edits of target %s (%s): %w", model.ShortID(targetID), notDeleted, err)
	}
	if len(current) > 0 {
		if model.SameEdits(current, edits) {
			return nil
		}
		return fmt.Errorf("target asset %s already carries different Immich edits; refusing to overwrite them (%s)", model.ShortID(targetID), notDeleted)
	}

	emitter.EmitProgress(model.ProgressEvent{AssetID: asset.ID, Filename: asset.OriginalFileName, Step: fmt.Sprintf("Carrying edits (%s) to %s...", model.DescribeEdits(edits), model.ShortID(targetID))})
	applied, err := u.Client.ReplaceAssetEdits(targetID, edits)
	if err != nil {
		if isPermissionDenied(err) {
			return fmt.Errorf("%w: applying edits was denied, the API key needs the asset.edit.create permission (%s): %w", errKeyPermission, notDeleted, err)
		}
		return fmt.Errorf("apply edits to %s (%s): %w", model.ShortID(targetID), notDeleted, err)
	}
	if !model.SameEdits(applied, edits) {
		return fmt.Errorf("target asset %s holds edits %q after carrying %q (%s)", model.ShortID(targetID), model.DescribeEdits(applied), model.DescribeEdits(edits), notDeleted)
	}
	return nil
}

func (u *ModernUploader) waitForDimensions(assetID string) (string, error) {
	for attempt := 1; ; attempt++ {
		width, height, orientation, err := u.Client.GetExifGeometry(assetID)
		if err != nil {
			return "", fmt.Errorf("check dimensions of %s: %w", model.ShortID(assetID), err)
		}
		if width > 0 && height > 0 {
			return uprightAsOne(orientation), nil
		}
		if attempt >= dimensionsWaitAttempts {
			return "", fmt.Errorf("Immich did not extract the dimensions of %s after %d checks, so its edits cannot be applied yet", model.ShortID(assetID), attempt)
		}
		sleepFn(time.Second)
	}
}

func editsReadError(err error) error {
	if isPermissionDenied(err) {
		return fmt.Errorf("reading Immich edits was denied, the API key needs the asset.edit.get permission: %w", err)
	}
	return fmt.Errorf("fetch edits: %w", err)
}

func fileSHA1(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	hasher := sha1.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return nil, err
	}
	return hasher.Sum(nil), nil
}

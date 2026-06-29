package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/exif"
	"github.com/majorfi/immich-exif/model"
)

func captureStdout(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func captureStderr(t *testing.T, runFn func()) string {
	t.Helper()
	originalStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	os.Stderr = writer

	runFn()

	if err := writer.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}
	os.Stderr = originalStderr

	outputBytes, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read stderr output: %v", err)
	}
	return string(outputBytes)
}

func withFakeStdio(stdinFile, stdoutFile *os.File) (func(), func()) {
	originalStdin := os.Stdin
	originalStdout := os.Stdout
	os.Stdin = stdinFile
	os.Stdout = stdoutFile
	return func() {
			os.Stdin = originalStdin
		}, func() {
			os.Stdout = originalStdout
		}
}

func setupConfigTest(args []string) func() {
	originalArgs := os.Args
	originalCommandLine := flag.CommandLine
	os.Args = args
	flag.CommandLine = flag.NewFlagSet(args[0], flag.ContinueOnError)
	return func() {
		os.Args = originalArgs
		flag.CommandLine = originalCommandLine
	}
}

func TestResolveAssetIDsDedupsPositionalIDs(t *testing.T) {
	cfg := &model.Config{
		AssetIDs: []string{"a", "b", "a", "c", "b"},
	}

	got, stats, err := resolveAssetIDs(nil, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.NoWritableMetadataSkipped != 0 {
		t.Fatalf("expected 0 noWritableMetadata, got %d", stats.NoWritableMetadataSkipped)
	}
	if stats.UnsupportedVideoSkipped != 0 {
		t.Fatalf("expected 0 unsupportedVideoSkipped, got %d", stats.UnsupportedVideoSkipped)
	}
	if stats.StateSkipped != 0 {
		t.Fatalf("expected 0 stateSkipped, got %d", stats.StateSkipped)
	}

	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("expected %d IDs, got %d (%v)", len(want), len(got), got)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected IDs %v, got %v", want, got)
		}
	}
}

func TestDedupPreservesOrder(t *testing.T) {
	got := dedup([]string{"c", "a", "b", "a", "c"})
	want := []string{"c", "a", "b"}
	if len(got) != len(want) {
		t.Fatalf("expected %d, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func TestDedupEmpty(t *testing.T) {
	got := dedup([]string{})
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestDedupSingleElement(t *testing.T) {
	got := dedup([]string{"a"})
	if len(got) != 1 || got[0] != "a" {
		t.Fatalf("expected [a], got %v", got)
	}
}

func TestDedupAllDuplicates(t *testing.T) {
	got := dedup([]string{"x", "x", "x"})
	if len(got) != 1 || got[0] != "x" {
		t.Fatalf("expected [x], got %v", got)
	}
}

func TestResolveAssetIDsWithAll(t *testing.T) {
	desc := "test"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := model.SearchMetadataResponse{
			Assets: model.SearchAssets{
				Items: []model.AssetResponse{
					{ID: "a1", ExifInfo: &model.ExifInfo{Description: &desc}},
					{ID: "a2", ExifInfo: &model.ExifInfo{Description: &desc}},
				},
				NextPage: nil,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := api.NewImmichClient(server.URL, "key")
	cfg := &model.Config{All: true}

	ids, _, err := resolveAssetIDs(client, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 IDs, got %d", len(ids))
	}
}

func TestResolveAssetIDsWithAlbum(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := model.AlbumResponse{
			ID:     "album-1",
			Assets: []model.AssetResponse{{ID: "b1"}, {ID: "b2"}, {ID: "b3"}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := api.NewImmichClient(server.URL, "key")
	cfg := &model.Config{AlbumIDs: []string{"album-1"}}

	ids, _, err := resolveAssetIDs(client, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("expected 3 IDs, got %d", len(ids))
	}
}

func TestResolveAssetIDsWithAllAlbums(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/albums" {
			resp := []model.AlbumResponse{
				{ID: "album-1"},
				{ID: "album-2"},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		if r.URL.Path == "/api/albums/album-1" {
			resp := model.AlbumResponse{
				ID:     "album-1",
				Assets: []model.AssetResponse{{ID: "a1"}, {ID: "a2"}},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		if r.URL.Path == "/api/albums/album-2" {
			resp := model.AlbumResponse{
				ID:     "album-2",
				Assets: []model.AssetResponse{{ID: "a2"}, {ID: "a3"}},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := api.NewImmichClient(server.URL, "key")
	cfg := &model.Config{AllAlbums: true}

	ids, _, err := resolveAssetIDs(client, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("expected 3 IDs, got %d", len(ids))
	}
	if ids[0] != "a1" || ids[1] != "a2" || ids[2] != "a3" {
		t.Fatalf("unexpected IDs: %v", ids)
	}
}

func TestResolveAssetIDsWithAllError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := api.NewImmichClient(server.URL, "key")
	cfg := &model.Config{All: true}

	_, _, err := resolveAssetIDs(client, cfg, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveAssetIDsWithAlbumError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := api.NewImmichClient(server.URL, "key")
	cfg := &model.Config{AlbumIDs: []string{"album-1"}}

	_, _, err := resolveAssetIDs(client, cfg, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveAssetIDsWithAllAlbumsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := api.NewImmichClient(server.URL, "key")
	cfg := &model.Config{AllAlbums: true}

	_, _, err := resolveAssetIDs(client, cfg, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveExportDirWithoutAlbum(t *testing.T) {
	cfg := &model.Config{ExportDir: "/tmp/export", AlbumIDs: nil}
	got := resolveExportDir(cfg)
	if got != "/tmp/export" {
		t.Fatalf("expected /tmp/export, got %s", got)
	}
}

func TestResolveExportDirWithSingleAlbum(t *testing.T) {
	cfg := &model.Config{ExportDir: "/tmp/export", AlbumIDs: []string{"album-123"}}
	got := resolveExportDir(cfg)
	want := filepath.Join("/tmp/export", "album-123")
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestResolveExportDirWithMultipleAlbums(t *testing.T) {
	cfg := &model.Config{ExportDir: "/tmp/export", AlbumIDs: []string{"album-1", "album-2"}}
	got := resolveExportDir(cfg)
	if got != "/tmp/export" {
		t.Fatalf("expected /tmp/export, got %s", got)
	}
}

func TestUnresolvedDuplicateResults(t *testing.T) {
	results := []model.ProcessResult{
		{AssetID: "a1", Status: model.StatusSkipped, DuplicateID: "d1"},
		{AssetID: "a1", Status: model.StatusSkipped, DuplicateID: "d1"},
		{AssetID: "a2", Status: model.StatusSkipped, DuplicateID: "d2"},
		{AssetID: "a3", Status: model.StatusSuccess, DuplicateID: "d3"},
		{AssetID: "a4", Status: model.StatusSkipped},
	}

	unresolved := unresolvedDuplicateResults(results)
	if len(unresolved) != 2 {
		t.Fatalf("expected 2 unresolved duplicates, got %d", len(unresolved))
	}
	if unresolved[0].AssetID != "a1" || unresolved[0].DuplicateID != "d1" {
		t.Fatalf("unexpected first unresolved result: %#v", unresolved[0])
	}
	if unresolved[1].AssetID != "a2" || unresolved[1].DuplicateID != "d2" {
		t.Fatalf("unexpected second unresolved result: %#v", unresolved[1])
	}
}

func TestBuildResolveDuplicateCommand(t *testing.T) {
	results := []model.ProcessResult{
		{AssetID: "asset-1", Status: model.StatusSkipped, DuplicateID: "dup-1"},
		{AssetID: "asset-2", Status: model.StatusSkipped, DuplicateID: "dup-2"},
		{AssetID: "asset-1", Status: model.StatusSkipped, DuplicateID: "dup-1"},
		{AssetID: "asset-3", Status: model.StatusSuccess, DuplicateID: "dup-3"},
	}

	command := buildResolveDuplicateCommand(results)
	want := "immich-exif -y -resolve-duplicate asset-1 asset-2"
	if command != want {
		t.Fatalf("expected %q, got %q", want, command)
	}
}

func TestUnresolvedDuplicateAssetIDs(t *testing.T) {
	results := []model.ProcessResult{
		{AssetID: "asset-1", Status: model.StatusSkipped, DuplicateID: "dup-1"},
		{AssetID: "asset-2", Status: model.StatusSkipped, DuplicateID: "dup-2"},
		{AssetID: "asset-1", Status: model.StatusSkipped, DuplicateID: "dup-1"},
		{AssetID: "asset-3", Status: model.StatusSuccess, DuplicateID: "dup-3"},
	}

	assetIDs := unresolvedDuplicateAssetIDs(results)
	if len(assetIDs) != 2 {
		t.Fatalf("expected 2 asset IDs, got %d", len(assetIDs))
	}
	if assetIDs[0] != "asset-1" || assetIDs[1] != "asset-2" {
		t.Fatalf("expected [asset-1 asset-2], got %v", assetIDs)
	}
}

func TestIsAffirmativeInput(t *testing.T) {
	if !isAffirmativeInput("yes") {
		t.Fatal("expected yes to be affirmative")
	}
	if !isAffirmativeInput("Y") {
		t.Fatal("expected Y to be affirmative")
	}
	if isAffirmativeInput("no") {
		t.Fatal("expected no to be negative")
	}
}

func TestPromptResolveDuplicatesNowYes(t *testing.T) {
	reader := strings.NewReader("yes\n")
	var writer bytes.Buffer

	confirmed := promptResolveDuplicatesNow(reader, &writer)
	if !confirmed {
		t.Fatal("expected confirmation to be true for yes")
	}
	output := writer.String()
	if !strings.Contains(output, "Patch unresolved duplicates now? [y/N]: ") {
		t.Fatalf("unexpected prompt output: %q", output)
	}
}

func TestPromptResolveDuplicatesNowNo(t *testing.T) {
	reader := strings.NewReader("no\n")
	var writer bytes.Buffer

	confirmed := promptResolveDuplicatesNow(reader, &writer)
	if confirmed {
		t.Fatal("expected confirmation to be false for no")
	}
}

func TestBuildResolveDuplicateFollowUpConfig(t *testing.T) {
	cfg := &model.Config{
		Yes:              false,
		ResolveDuplicate: false,
		Workers:          4,
	}

	resolveCfg := buildResolveDuplicateFollowUpConfig(cfg)
	if !resolveCfg.Yes {
		t.Fatal("expected follow-up config to enable yes")
	}
	if !resolveCfg.ResolveDuplicate {
		t.Fatal("expected follow-up config to enable resolveDuplicate")
	}
	if resolveCfg.Workers != 4 {
		t.Fatalf("expected workers to be preserved, got %d", resolveCfg.Workers)
	}
	if cfg.Yes || cfg.ResolveDuplicate {
		t.Fatal("original config should remain unchanged")
	}
}

func TestPrintUnresolvedDuplicateHintWithResults(t *testing.T) {
	results := []model.ProcessResult{
		{AssetID: "asset-1", Status: model.StatusSkipped, DuplicateID: "dup-1"},
		{AssetID: "asset-2", Status: model.StatusSkipped, DuplicateID: "dup-2"},
	}

	output := captureStdout(func() {
		printUnresolvedDuplicateHint(results)
	})

	if !strings.Contains(output, "Skipped duplicate uploads (2):") {
		t.Fatalf("missing skipped summary: %q", output)
	}
	if !strings.Contains(output, "asset-1 -> dup-1") {
		t.Fatalf("missing first duplicate pair: %q", output)
	}
	if !strings.Contains(output, "To patch them automatically, rerun with the same auth flags or environment and:") {
		t.Fatalf("missing follow-up command hint: %q", output)
	}
	if !strings.Contains(output, "immich-exif -y -resolve-duplicate asset-1 asset-2") {
		t.Fatalf("missing follow-up command: %q", output)
	}
}

func TestPrintUnresolvedDuplicateHintWithoutResults(t *testing.T) {
	output := captureStdout(func() {
		printUnresolvedDuplicateHint([]model.ProcessResult{
			{AssetID: "asset-1", Status: model.StatusSuccess},
		})
	})

	if strings.TrimSpace(output) != "" {
		t.Fatalf("expected empty output, got %q", output)
	}
}

func TestMaybeResolveDuplicatesNowEarlyExit(t *testing.T) {
	results := []model.ProcessResult{
		{AssetID: "asset-1", Status: model.StatusSkipped, DuplicateID: "dup-1"},
	}

	yesCfg := &model.Config{Yes: true}
	if followUp := maybeResolveDuplicatesNow(context.Background(), nil, yesCfg, results); followUp != nil {
		t.Fatalf("expected nil follow-up with -y, got %v", followUp)
	}

	resolveCfg := &model.Config{ResolveDuplicate: true}
	if followUp := maybeResolveDuplicatesNow(context.Background(), nil, resolveCfg, results); followUp != nil {
		t.Fatalf("expected nil follow-up with -resolve-duplicate, got %v", followUp)
	}
}

func TestMaybeResolveDuplicatesNowNoUnresolved(t *testing.T) {
	cfg := &model.Config{}
	results := []model.ProcessResult{
		{AssetID: "asset-1", Status: model.StatusSuccess},
	}
	if followUp := maybeResolveDuplicatesNow(context.Background(), nil, cfg, results); followUp != nil {
		t.Fatalf("expected nil follow-up without unresolved duplicates, got %v", followUp)
	}
}

func TestMaybeResolveDuplicatesNowNonTerminal(t *testing.T) {
	cfg := &model.Config{}
	results := []model.ProcessResult{
		{AssetID: "asset-1", Status: model.StatusSkipped, DuplicateID: "dup-1"},
	}

	tempFilePath := filepath.Join(t.TempDir(), "stdio.txt")
	tempFile, err := os.Create(tempFilePath)
	if err != nil {
		t.Fatalf("create temp stdio file: %v", err)
	}
	defer tempFile.Close()

	restoreStdin, restoreStdout := withFakeStdio(tempFile, tempFile)
	defer restoreStdin()
	defer restoreStdout()

	if followUp := maybeResolveDuplicatesNow(context.Background(), nil, cfg, results); followUp != nil {
		t.Fatalf("expected nil follow-up for non-terminal stdio, got %v", followUp)
	}
}

func TestMaybeResolveDuplicatesNowTerminalPromptEOF(t *testing.T) {
	cfg := &model.Config{}
	results := []model.ProcessResult{
		{AssetID: "asset-1", Status: model.StatusSkipped, DuplicateID: "dup-1"},
	}

	stdinFile, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open devnull stdin: %v", err)
	}
	defer stdinFile.Close()

	stdoutFile, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open devnull stdout: %v", err)
	}
	defer stdoutFile.Close()

	restoreStdin, restoreStdout := withFakeStdio(stdinFile, stdoutFile)
	defer restoreStdin()
	defer restoreStdout()

	if followUp := maybeResolveDuplicatesNow(context.Background(), nil, cfg, results); followUp != nil {
		t.Fatalf("expected nil follow-up for EOF prompt input, got %v", followUp)
	}
}

func TestIsTerminalFile(t *testing.T) {
	if isTerminalFile(nil) {
		t.Fatal("nil file must not be terminal")
	}

	regularFilePath := filepath.Join(t.TempDir(), "regular.txt")
	if err := os.WriteFile(regularFilePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("write regular file: %v", err)
	}
	regularFile, err := os.Open(regularFilePath)
	if err != nil {
		t.Fatalf("open regular file: %v", err)
	}
	defer regularFile.Close()

	if isTerminalFile(regularFile) {
		t.Fatal("regular file must not be terminal")
	}
}

func TestAddAlbumAssetMappings(t *testing.T) {
	exportAlbumIDsByAsset := map[string][]string{}

	addAlbumAssetMappings(exportAlbumIDsByAsset, []string{"a1", "a2"}, "album-1")
	addAlbumAssetMappings(exportAlbumIDsByAsset, []string{"a2", "a3"}, "album-2")
	addAlbumAssetMappings(exportAlbumIDsByAsset, []string{"a1", "a2"}, "album-1")

	if len(exportAlbumIDsByAsset["a1"]) != 1 || exportAlbumIDsByAsset["a1"][0] != "album-1" {
		t.Fatalf("unexpected album mapping for a1: %v", exportAlbumIDsByAsset["a1"])
	}
	if len(exportAlbumIDsByAsset["a2"]) != 2 {
		t.Fatalf("expected two albums for a2, got %v", exportAlbumIDsByAsset["a2"])
	}
	if exportAlbumIDsByAsset["a2"][0] != "album-1" || exportAlbumIDsByAsset["a2"][1] != "album-2" {
		t.Fatalf("unexpected album mapping for a2: %v", exportAlbumIDsByAsset["a2"])
	}
	if len(exportAlbumIDsByAsset["a3"]) != 1 || exportAlbumIDsByAsset["a3"][0] != "album-2" {
		t.Fatalf("unexpected album mapping for a3: %v", exportAlbumIDsByAsset["a3"])
	}
}

func TestResolveAssetIDsBuildsExportAlbumMappingsForAllAlbums(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/albums" {
			resp := []model.AlbumResponse{{ID: "album-1"}, {ID: "album-2"}}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		if r.URL.Path == "/api/albums/album-1" {
			resp := model.AlbumResponse{ID: "album-1", Assets: []model.AssetResponse{{ID: "a1"}, {ID: "a2"}}}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		if r.URL.Path == "/api/albums/album-2" {
			resp := model.AlbumResponse{ID: "album-2", Assets: []model.AssetResponse{{ID: "a2"}, {ID: "a3"}}}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := api.NewImmichClient(server.URL, "key")
	cfg := &model.Config{
		AllAlbums: true,
		ExportDir: t.TempDir(),
	}

	ids, _, err := resolveAssetIDs(client, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("expected 3 deduped IDs, got %d (%v)", len(ids), ids)
	}
	if len(cfg.ExportAlbumIDsByAsset["a2"]) != 2 {
		t.Fatalf("expected mirrored mapping for a2, got %v", cfg.ExportAlbumIDsByAsset["a2"])
	}
	if cfg.ExportAlbumIDsByAsset["a2"][0] != "album-1" || cfg.ExportAlbumIDsByAsset["a2"][1] != "album-2" {
		t.Fatalf("unexpected album list for a2: %v", cfg.ExportAlbumIDsByAsset["a2"])
	}
}

func TestResolveAssetIDsDoesNotBuildExportAlbumMappingsForSingleAlbum(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := model.AlbumResponse{ID: "album-1", Assets: []model.AssetResponse{{ID: "a1"}}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := api.NewImmichClient(server.URL, "key")
	cfg := &model.Config{
		AlbumIDs:  []string{"album-1"},
		ExportDir: t.TempDir(),
	}

	_, _, err := resolveAssetIDs(client, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ExportAlbumIDsByAsset != nil {
		t.Fatalf("expected no per-asset album mapping for single album, got %v", cfg.ExportAlbumIDsByAsset)
	}
}

func TestResolveAssetIDsMirrorsAllAssetsAndIncludesNoAlbumBucket(t *testing.T) {
	desc := "test"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/search/metadata" {
			resp := model.SearchMetadataResponse{
				Assets: model.SearchAssets{
					Items: []model.AssetResponse{
						{ID: "album-asset", ExifInfo: &model.ExifInfo{Description: &desc}},
						{ID: "shared-asset", ExifInfo: &model.ExifInfo{Description: &desc}},
						{ID: "lonely-asset", ExifInfo: &model.ExifInfo{Description: &desc}},
					},
					NextPage: nil,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		if r.URL.Path == "/api/albums" {
			resp := []model.AlbumResponse{{ID: "album-1"}, {ID: "album-2"}}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		if r.URL.Path == "/api/albums/album-1" {
			resp := model.AlbumResponse{ID: "album-1", Assets: []model.AssetResponse{{ID: "album-asset"}, {ID: "shared-asset"}}}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		if r.URL.Path == "/api/albums/album-2" {
			resp := model.AlbumResponse{ID: "album-2", Assets: []model.AssetResponse{{ID: "shared-asset"}}}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := api.NewImmichClient(server.URL, "key")
	cfg := &model.Config{
		All:            true,
		AllAlbums:      true,
		IncludeNoAlbum: true,
		ExportDir:      t.TempDir(),
	}

	ids, _, err := resolveAssetIDs(client, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("expected 3 IDs, got %d (%v)", len(ids), ids)
	}
	if len(cfg.ExportAlbumIDsByAsset["shared-asset"]) != 2 {
		t.Fatalf("expected shared asset in 2 album folders, got %v", cfg.ExportAlbumIDsByAsset["shared-asset"])
	}
	if cfg.ExportAlbumIDsByAsset["lonely-asset"][0] != noAlbumDirName {
		t.Fatalf("expected lonely asset in %s, got %v", noAlbumDirName, cfg.ExportAlbumIDsByAsset["lonely-asset"])
	}
}

func TestResolveAssetIDsMirrorsAllAssetsWithoutNoAlbumBucketWhenDisabled(t *testing.T) {
	desc := "test"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/search/metadata" {
			resp := model.SearchMetadataResponse{
				Assets: model.SearchAssets{
					Items: []model.AssetResponse{
						{ID: "album-asset", ExifInfo: &model.ExifInfo{Description: &desc}},
						{ID: "lonely-asset", ExifInfo: &model.ExifInfo{Description: &desc}},
					},
					NextPage: nil,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		if r.URL.Path == "/api/albums" {
			resp := []model.AlbumResponse{{ID: "album-1"}}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		if r.URL.Path == "/api/albums/album-1" {
			resp := model.AlbumResponse{ID: "album-1", Assets: []model.AssetResponse{{ID: "album-asset"}}}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := api.NewImmichClient(server.URL, "key")
	cfg := &model.Config{
		All:            true,
		AllAlbums:      true,
		IncludeNoAlbum: false,
		ExportDir:      t.TempDir(),
	}

	ids, _, err := resolveAssetIDs(client, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 1 || ids[0] != "album-asset" {
		t.Fatalf("expected only album asset, got %v", ids)
	}
	if _, exists := cfg.ExportAlbumIDsByAsset["lonely-asset"]; exists {
		t.Fatalf("expected lonely asset to be omitted, got %v", cfg.ExportAlbumIDsByAsset["lonely-asset"])
	}
}

func TestRunPipelineForcesSingleWorkerInInteractiveMode(t *testing.T) {
	cfg := &model.Config{Workers: 4, Yes: false}

	output := captureStdout(func() {
		results := runPipeline(context.Background(), nil, nil, cfg, []string{})
		if len(results) != 0 {
			t.Fatalf("expected no results for empty asset list, got %d", len(results))
		}
	})

	if cfg.Workers != 1 {
		t.Fatalf("expected workers to be forced to 1, got %d", cfg.Workers)
	}
	if !strings.Contains(output, "Interactive mode forces single worker (requested: 4)") {
		t.Fatalf("expected interactive worker warning, got %q", output)
	}
}

func TestRunPipelineKeepsWorkerCountWhenAutoConfirmEnabled(t *testing.T) {
	cfg := &model.Config{Workers: 4, Yes: true}

	output := captureStdout(func() {
		results := runPipeline(context.Background(), nil, nil, cfg, []string{})
		if len(results) != 0 {
			t.Fatalf("expected no results for empty asset list, got %d", len(results))
		}
	})

	if cfg.Workers != 4 {
		t.Fatalf("expected workers to stay at 4, got %d", cfg.Workers)
	}
	if strings.Contains(output, "Interactive mode forces single worker") {
		t.Fatalf("did not expect interactive worker warning, got %q", output)
	}
}

func TestFinalUnresolvedDuplicateResultsUsesLatestAttempt(t *testing.T) {
	results := []model.ProcessResult{
		{AssetID: "asset-1", Status: model.StatusSkipped, DuplicateID: "dup-1"},
		{AssetID: "asset-1", Status: model.StatusSuccess, NewID: "dup-1"},
		{AssetID: "asset-2", Status: model.StatusSkipped, DuplicateID: "dup-2"},
	}

	unresolved := finalUnresolvedDuplicateResults(results)
	if len(unresolved) != 1 {
		t.Fatalf("expected 1 unresolved duplicate, got %d", len(unresolved))
	}
	if unresolved[0].AssetID != "asset-2" {
		t.Fatalf("expected unresolved asset-2, got %s", unresolved[0].AssetID)
	}
}

func TestFinalUnresolvedDuplicateResultsKeepsFinalSkippedDuplicate(t *testing.T) {
	results := []model.ProcessResult{
		{AssetID: "asset-1", Status: model.StatusSkipped, DuplicateID: "dup-1"},
		{AssetID: "asset-1", Status: model.StatusSkipped, DuplicateID: "dup-2"},
		{AssetID: "asset-2", Status: model.StatusSuccess, NewID: "new-2"},
	}

	unresolved := finalUnresolvedDuplicateResults(results)
	if len(unresolved) != 1 {
		t.Fatalf("expected 1 unresolved duplicate, got %d", len(unresolved))
	}
	if unresolved[0].DuplicateID != "dup-2" {
		t.Fatalf("expected latest duplicate ID dup-2, got %s", unresolved[0].DuplicateID)
	}
}

func TestRememberAssetSnapshotStoresSnapshot(t *testing.T) {
	originalSnapshotAssetFn := snapshotAssetFn
	snapshotAssetFn = func(asset model.AssetResponse) (string, error) {
		return "snap-" + asset.ID, nil
	}
	t.Cleanup(func() {
		snapshotAssetFn = originalSnapshotAssetFn
	})

	snapshots := map[string]string{}
	snapshot, ok := rememberAssetSnapshot(model.AssetResponse{ID: "asset-1"}, snapshots)
	if !ok {
		t.Fatal("expected snapshot capture success")
	}
	if snapshot != "snap-asset-1" {
		t.Fatalf("unexpected snapshot %q", snapshot)
	}
	if snapshots["asset-1"] != "snap-asset-1" {
		t.Fatalf("expected saved snapshot, got %#v", snapshots)
	}
}

func TestRememberAssetSnapshotWarnsOnError(t *testing.T) {
	originalSnapshotAssetFn := snapshotAssetFn
	snapshotAssetFn = func(asset model.AssetResponse) (string, error) {
		return "", fmt.Errorf("boom")
	}
	t.Cleanup(func() {
		snapshotAssetFn = originalSnapshotAssetFn
	})

	snapshots := map[string]string{}
	output := captureStderr(t, func() {
		snapshot, ok := rememberAssetSnapshot(model.AssetResponse{ID: "asset-1"}, snapshots)
		if ok {
			t.Fatal("expected snapshot capture failure")
		}
		if snapshot != "" {
			t.Fatalf("expected empty snapshot, got %q", snapshot)
		}
	})

	if len(snapshots) != 0 {
		t.Fatalf("expected no snapshots saved, got %#v", snapshots)
	}
	if !strings.Contains(output, "Warning: failed to snapshot asset asset-1") {
		t.Fatalf("expected warning output, got %q", output)
	}
}

func TestSetupTmpDirCreatesUniqueDirectoryInWorkingDir(t *testing.T) {
	workingDir := t.TempDir()
	originalWorkingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get current working directory: %v", err)
	}
	if err := os.Chdir(workingDir); err != nil {
		t.Fatalf("change to temp working directory: %v", err)
	}
	t.Cleanup(func() {
		os.Chdir(originalWorkingDir)
	})
	resolvedWorkingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve temp working directory: %v", err)
	}

	existingTmpDir := filepath.Join(workingDir, ".immich-exif-tmp")
	if err := os.MkdirAll(existingTmpDir, 0o755); err != nil {
		t.Fatalf("create existing tmp dir: %v", err)
	}
	oldFilePath := filepath.Join(existingTmpDir, "old.txt")
	if err := os.WriteFile(oldFilePath, []byte("old"), 0o644); err != nil {
		t.Fatalf("create old file in tmp dir: %v", err)
	}

	firstTmpDir, err := setupTmpDir()
	if err != nil {
		t.Fatalf("setup tmp dir: %v", err)
	}
	secondTmpDir, err := setupTmpDir()
	if err != nil {
		t.Fatalf("setup second tmp dir: %v", err)
	}
	if firstTmpDir == secondTmpDir {
		t.Fatalf("expected unique tmp directories, got %q", firstTmpDir)
	}

	prefix := filepath.Join(resolvedWorkingDir, ".immich-exif-tmp-")
	if !strings.HasPrefix(firstTmpDir, prefix) {
		t.Fatalf("expected tmp dir prefix %q, got %q", prefix, firstTmpDir)
	}

	dirInfo, err := os.Stat(firstTmpDir)
	if err != nil {
		t.Fatalf("stat tmp dir: %v", err)
	}
	if !dirInfo.IsDir() {
		t.Fatalf("tmp path is not a directory: %q", firstTmpDir)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("expected tmp dir permissions 0700, got %o", dirInfo.Mode().Perm())
	}

	oldFileContent, err := os.ReadFile(oldFilePath)
	if err != nil {
		t.Fatalf("expected previous tmp content to remain untouched, err=%v", err)
	}
	if string(oldFileContent) != "old" {
		t.Fatalf("unexpected previous tmp content: %q", string(oldFileContent))
	}
}

func TestShouldUseStateCache(t *testing.T) {
	testCases := []struct {
		name string
		cfg  *model.Config
		want bool
	}{
		{name: "nil config", cfg: nil, want: false},
		{name: "not all", cfg: &model.Config{}, want: false},
		{name: "all upload mode", cfg: &model.Config{All: true}, want: true},
		{name: "all dry run", cfg: &model.Config{All: true, DryRun: true}, want: false},
		{name: "all export mode", cfg: &model.Config{All: true, ExportDir: "/tmp/out"}, want: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := shouldUseStateCache(testCase.cfg)
			if got != testCase.want {
				t.Fatalf("expected %v, got %v", testCase.want, got)
			}
		})
	}
}

func TestParseConfigRejectsMissingURL(t *testing.T) {
	oldURL := os.Getenv("IMMICH_URL")
	os.Setenv("IMMICH_URL", "")
	defer os.Setenv("IMMICH_URL", oldURL)

	defer setupConfigTest([]string{
		"immich-exif",
		"-api-key", "test-key",
		"-all",
	})()

	_, err := parseConfig()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "url") || !strings.Contains(err.Error(), "IMMICH_URL") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseConfigRejectsMissingAPIKey(t *testing.T) {
	oldKey := os.Getenv("IMMICH_API_KEY")
	os.Setenv("IMMICH_API_KEY", "")
	defer os.Setenv("IMMICH_API_KEY", oldKey)

	defer setupConfigTest([]string{
		"immich-exif",
		"-url", "https://example.com",
		"-all",
	})()

	_, err := parseConfig()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "api-key") || !strings.Contains(err.Error(), "IMMICH_API_KEY") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseConfigRejectsMissingSelector(t *testing.T) {
	defer setupConfigTest([]string{
		"immich-exif",
		"-url", "https://example.com",
		"-api-key", "test-key",
	})()

	_, err := parseConfig()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "specify one selector") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseConfigRejectsBlankAlbum(t *testing.T) {
	defer setupConfigTest([]string{
		"immich-exif",
		"-url", "https://example.com",
		"-api-key", "test-key",
		"-album", "  ",
	})()

	_, err := parseConfig()
	if err == nil {
		t.Fatal("expected error for blank album")
	}
	if !strings.Contains(err.Error(), "album value cannot be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseConfigRejectsCombinedSelectors(t *testing.T) {
	defer setupConfigTest([]string{
		"immich-exif",
		"-url", "https://example.com",
		"-api-key", "test-key",
		"-all",
		"asset-id-1",
	})()

	_, err := parseConfig()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseConfigSuccess(t *testing.T) {
	defer setupConfigTest([]string{
		"immich-exif",
		"-url", "https://example.com/",
		"-api-key", "test-key",
		"-workers", "3",
		"-dry-run",
		"-all",
	})()

	cfg, err := parseConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.URL != "https://example.com" {
		t.Fatalf("expected trailing slash stripped, got %s", cfg.URL)
	}
	if cfg.APIKey != "test-key" {
		t.Fatalf("expected test-key, got %s", cfg.APIKey)
	}
	if cfg.Workers != 3 {
		t.Fatalf("expected 3 workers, got %d", cfg.Workers)
	}
	if !cfg.DryRun {
		t.Fatal("expected dry-run=true")
	}
	if !cfg.All {
		t.Fatal("expected all=true")
	}
	if !cfg.AllAlbums {
		t.Fatal("expected allAlbums=true")
	}
	if !cfg.IncludeNoAlbum {
		t.Fatal("expected includeNoAlbum=true")
	}
}

func TestParseConfigVersionFlag(t *testing.T) {
	defer setupConfigTest([]string{"immich-exif", "-version"})()

	if _, err := parseConfig(); !errors.Is(err, errShowVersion) {
		t.Fatalf("expected errShowVersion for -version, got %v", err)
	}
}

func TestWarnCredentialHygieneFlagsApiKeyFlag(t *testing.T) {
	defer setupConfigTest([]string{"immich-exif", "-url", "https://example.com", "-api-key", "secret", "-all"})()

	if _, err := parseConfig(); err != nil {
		t.Fatalf("parseConfig: %v", err)
	}

	out := captureStderr(t, func() { warnCredentialHygiene() })
	if !strings.Contains(out, "--api-key") {
		t.Fatalf("expected an --api-key hygiene warning, got %q", out)
	}
}

func TestParseConfigVerifyUploadDefaultsOn(t *testing.T) {
	defer setupConfigTest([]string{
		"immich-exif",
		"-url", "https://example.com",
		"-api-key", "test-key",
		"-all",
	})()

	cfg, err := parseConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.VerifyUpload {
		t.Fatal("expected verify-upload to default to true (safe by default)")
	}
}

func TestParseConfigNoVerifyUploadDisables(t *testing.T) {
	defer setupConfigTest([]string{
		"immich-exif",
		"-url", "https://example.com",
		"-api-key", "test-key",
		"-no-verify-upload",
		"-all",
	})()

	cfg, err := parseConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.VerifyUpload {
		t.Fatal("expected -no-verify-upload to disable verification")
	}
}

func TestParseConfigRejectsPlaintextHTTP(t *testing.T) {
	defer setupConfigTest([]string{
		"immich-exif",
		"-url", "http://example.com",
		"-api-key", "test-key",
		"-all",
	})()

	if _, err := parseConfig(); err == nil {
		t.Fatal("expected plaintext http:// URL to be rejected without --allow-http")
	}
}

func TestParseConfigAllowsHTTPWithFlag(t *testing.T) {
	defer setupConfigTest([]string{
		"immich-exif",
		"-url", "http://example.com",
		"-api-key", "test-key",
		"-allow-http",
		"-all",
	})()

	if _, err := parseConfig(); err != nil {
		t.Fatalf("expected http:// URL accepted with --allow-http, got: %v", err)
	}
}

func TestParseConfigWorkersMinimum(t *testing.T) {
	defer setupConfigTest([]string{
		"immich-exif",
		"-url", "https://example.com",
		"-api-key", "test-key",
		"-workers", "-5",
		"-all",
	})()

	cfg, err := parseConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Workers != 1 {
		t.Fatalf("expected workers clamped to 1, got %d", cfg.Workers)
	}
}

func TestParseConfigForceFlag(t *testing.T) {
	defer setupConfigTest([]string{
		"immich-exif",
		"-url", "https://example.com",
		"-api-key", "test-key",
		"-all",
		"-force",
	})()

	cfg, err := parseConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Force {
		t.Fatal("expected force=true")
	}
}

func TestParseConfigResolveDuplicateFlag(t *testing.T) {
	defer setupConfigTest([]string{
		"immich-exif",
		"-url", "https://example.com",
		"-api-key", "test-key",
		"-album", "album-1",
		"-resolve-duplicate",
	})()

	cfg, err := parseConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.ResolveDuplicate {
		t.Fatal("expected resolveDuplicate=true")
	}
}

func TestParseConfigAlbumAllSelector(t *testing.T) {
	defer setupConfigTest([]string{
		"immich-exif",
		"-url", "https://example.com",
		"-api-key", "test-key",
		"-album", "all",
	})()

	cfg, err := parseConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.AllAlbums {
		t.Fatal("expected allAlbums=true")
	}
	if !cfg.All {
		t.Fatal("expected all=true for album all alias")
	}
	if len(cfg.AlbumIDs) != 0 {
		t.Fatalf("expected no explicit album IDs, got %v", cfg.AlbumIDs)
	}
}

func TestParseConfigAllowsForceWithAlbumAllAlias(t *testing.T) {
	defer setupConfigTest([]string{
		"immich-exif",
		"-url", "https://example.com",
		"-api-key", "test-key",
		"-album", "all",
		"-force",
	})()

	cfg, err := parseConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Force {
		t.Fatal("expected force=true")
	}
}

func TestParseConfigRejectsAlbumAllWithOtherAlbums(t *testing.T) {
	defer setupConfigTest([]string{
		"immich-exif",
		"-url", "https://example.com",
		"-api-key", "test-key",
		"-album", "all",
		"-album", "album-1",
	})()

	_, err := parseConfig()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--album all cannot be combined with other album IDs") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseConfigRejectsForceWithoutAll(t *testing.T) {
	defer setupConfigTest([]string{
		"immich-exif",
		"-url", "https://example.com",
		"-api-key", "test-key",
		"-force",
		"asset-1",
	})()

	_, err := parseConfig()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--force can only be used with --all or --album all") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStringSlice(t *testing.T) {
	var s stringSlice
	if s.String() != "" {
		t.Fatalf("expected empty string, got %q", s.String())
	}
	s.Set("a")
	s.Set("b")
	if s.String() != "a,b" {
		t.Fatalf("expected a,b, got %q", s.String())
	}
}

func TestRunVersionFlag(t *testing.T) {
	defer setupConfigTest([]string{"immich-exif", "-version"})()
	var code int
	out := captureStdout(func() { code = run() })
	if code != 0 {
		t.Fatalf("expected exit 0 for -version, got %d", code)
	}
	if !strings.Contains(out, "immich-exif") {
		t.Fatalf("expected version banner, got %q", out)
	}
}

func TestRunConfigError(t *testing.T) {
	defer setupConfigTest([]string{"immich-exif"})()
	var code int
	captureStderr(t, func() { code = run() })
	if code != 1 {
		t.Fatalf("expected exit 1 on invalid config, got %d", code)
	}
}

func TestMaybeResolveDuplicatesNowCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg := &model.Config{}
	results := []model.ProcessResult{{AssetID: "a1", Status: model.StatusSkipped, DuplicateID: "d1"}}
	if got := maybeResolveDuplicatesNow(ctx, nil, cfg, results); got != nil {
		t.Fatalf("expected nil follow-up when the context is cancelled, got %v", got)
	}
}

func TestRunDryRunHappyPath(t *testing.T) {
	origCheck := exif.CheckExiftoolFn
	origRead := exif.ReadExifTagsFn
	origWrite := exif.WriteExifTagsFn
	exif.CheckExiftoolFn = func() error { return nil }
	exif.ReadExifTagsFn = func(string) (exif.ExifTagMap, error) { return exif.ExifTagMap{}, nil }
	exif.WriteExifTagsFn = func(string, []string) error { return nil }
	defer func() {
		exif.CheckExiftoolFn = origCheck
		exif.ReadExifTagsFn = origRead
		exif.WriteExifTagsFn = origWrite
	}()

	desc := "a description"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/server/about":
			json.NewEncoder(w).Encode(model.ServerAbout{Version: "v2.5.6"})
		case strings.HasSuffix(r.URL.Path, "/original"):
			w.Write([]byte("imagedata"))
		default:
			json.NewEncoder(w).Encode(model.AssetResponse{ID: "a1", OriginalFileName: "p.jpg", ExifInfo: &model.ExifInfo{Description: &desc}})
		}
	}))
	defer server.Close()

	defer setupConfigTest([]string{"immich-exif", "-url", server.URL, "-api-key", "k", "-allow-http", "-y", "-dry-run", "a1"})()
	var code int
	captureStdout(func() { code = run() })
	if code != 0 {
		t.Fatalf("expected exit 0 for a dry-run, got %d", code)
	}
}

func TestParseConfigListAlbumsNeedsNoSelector(t *testing.T) {
	defer setupConfigTest([]string{"immich-exif", "-url", "https://example.com", "-api-key", "k", "-list-albums"})()

	cfg, err := parseConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.ListAlbums {
		t.Fatal("expected ListAlbums=true")
	}
}

func TestRunListAlbums(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/server/about":
			json.NewEncoder(w).Encode(model.ServerAbout{Version: "v2.5.6"})
		case "/api/albums":
			json.NewEncoder(w).Encode([]model.AlbumResponse{
				{ID: "alb-1", AlbumName: "Vacation", AssetCount: 12},
				{ID: "alb-2", AlbumName: "Family", AssetCount: 3},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	defer setupConfigTest([]string{"immich-exif", "-url", server.URL, "-api-key", "k", "-allow-http", "-list-albums"})()
	var code int
	out := captureStdout(func() { code = run() })
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(out, "alb-1") || !strings.Contains(out, "Vacation") {
		t.Fatalf("expected album listing in output, got %q", out)
	}
}

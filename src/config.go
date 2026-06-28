package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"

	"github.com/majorfi/immich-exif/model"
)

type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ",") }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func parseConfig() (*model.Config, error) {
	if err := godotenv.Load(); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("load .env: %w", err)
		}
	}

	cfg := &model.Config{}
	var albums stringSlice
	var noVerifyUpload bool
	var allowHTTP bool

	flag.StringVar(&cfg.URL, "url", os.Getenv("IMMICH_URL"), "Immich server URL (env: IMMICH_URL)")
	flag.StringVar(&cfg.APIKey, "api-key", os.Getenv("IMMICH_API_KEY"), "API key (env: IMMICH_API_KEY)")
	flag.StringVar(&cfg.ImmichAPI, "immich-api", "auto", "Immich API contract: auto (detect), legacy, or v3")
	flag.IntVar(&cfg.Workers, "workers", 1, "Number of parallel workers")
	flag.BoolVar(&cfg.DryRun, "dry-run", false, "Embed EXIF but skip re-upload")
	flag.StringVar(&cfg.ExportDir, "export-dir", "", "Save files to directory instead of re-uploading")
	flag.BoolVar(&cfg.Yes, "y", false, "Auto-confirm all changes")
	flag.BoolVar(&noVerifyUpload, "no-verify-upload", false, "Skip checksum verification; the original is moved to Immich trash instead of being permanently deleted")
	flag.BoolVar(&allowHTTP, "allow-http", false, "Allow a plaintext http:// server URL (the API key is sent in clear text)")

	flag.BoolVar(&cfg.ResolveDuplicate, "resolve-duplicate", false, "Resolve duplicate upload status by copying associations to duplicate asset and deleting old asset")
	flag.BoolVar(&cfg.IncludeNoAlbum, "include-no-album", true, "With album-mirrored export, include assets with no album under no-album/")
	flag.BoolVar(&cfg.All, "all", false, "Process all assets")
	flag.BoolVar(&cfg.Force, "force", false, "Force re-processing all assets (ignore state cache)")
	flag.Var(&albums, "album", "Album ID to process (repeatable), or all")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: immich-exif [flags] [asset-ids...]\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}

	flag.Parse()
	cfg.VerifyUpload = !noVerifyUpload
	cfg.AlbumIDs = albums
	cfg.AssetIDs = flag.Args()
	if hasAllAlbumSelector(cfg.AlbumIDs) {
		if len(cfg.AlbumIDs) > 1 {
			return nil, fmt.Errorf("--album all cannot be combined with other album IDs")
		}
		cfg.AllAlbums = true
		cfg.All = true
		cfg.AlbumIDs = nil
	}

	if cfg.All {
		cfg.AllAlbums = true
	}

	for _, albumID := range cfg.AlbumIDs {
		if strings.TrimSpace(albumID) == "" {
			return nil, fmt.Errorf("--album value cannot be empty")
		}
	}

	switch cfg.ImmichAPI {
	case "auto", "legacy", "v3":
	default:
		return nil, fmt.Errorf("--immich-api must be one of: auto, legacy, v3")
	}

	if cfg.URL == "" {
		return nil, fmt.Errorf("--url or IMMICH_URL is required")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("--api-key or IMMICH_API_KEY is required")
	}

	cfg.URL = strings.TrimRight(cfg.URL, "/")

	if strings.HasPrefix(strings.ToLower(cfg.URL), "http://") && !allowHTTP {
		return nil, fmt.Errorf("refusing to send the API key over plaintext http:// (%s); use https:// or pass --allow-http to override", cfg.URL)
	}

	selectionModes := 0
	if cfg.All || cfg.AllAlbums {
		selectionModes++
	}
	if len(cfg.AlbumIDs) > 0 {
		selectionModes++
	}
	if len(cfg.AssetIDs) > 0 {
		selectionModes++
	}

	if selectionModes == 0 {
		return nil, fmt.Errorf("specify one selector: --all/--album all, --album <id>, or asset IDs")
	}
	if selectionModes > 1 {
		return nil, fmt.Errorf("selectors cannot be combined: choose exactly one of --all/--album all, --album <id>, or asset IDs")
	}

	if cfg.Force && !cfg.All {
		return nil, fmt.Errorf("--force can only be used with --all or --album all")
	}

	if cfg.Workers < 1 {
		cfg.Workers = 1
	}

	return cfg, nil
}

func hasAllAlbumSelector(albumIDs []string) bool {
	for _, albumID := range albumIDs {
		if strings.EqualFold(strings.TrimSpace(albumID), "all") {
			return true
		}
	}
	return false
}

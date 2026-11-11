package model

import (
	"path"
	"path/filepath"
	"strings"
	"time"
)

type AssetResponse struct {
	ID               string           `json:"id"`
	DeviceAssetID    string           `json:"deviceAssetId"`
	DeviceID         string           `json:"deviceId"`
	OriginalFileName string           `json:"originalFileName"`
	OriginalPath     string           `json:"originalPath"`
	OriginalMimeType string           `json:"originalMimeType"`
	Checksum         string           `json:"checksum"`
	FileCreatedAt    time.Time        `json:"fileCreatedAt"`
	FileModifiedAt   time.Time        `json:"fileModifiedAt"`
	UpdatedAt        time.Time        `json:"updatedAt"`
	IsFavorite       bool             `json:"isFavorite"`
	IsArchived       bool             `json:"isArchived"`
	IsTrashed        bool             `json:"isTrashed"`
	IsEdited         bool             `json:"isEdited"`
	Visibility       string           `json:"visibility"`
	LivePhotoVideoID string           `json:"livePhotoVideoId"`
	LibraryID        string           `json:"libraryId"`
	ExifInfo         *ExifInfo        `json:"exifInfo"`
	People           []PersonResponse `json:"people"`
	Stack            *AssetStack      `json:"stack"`
	Tags             []TagResponse    `json:"tags"`
}

type TagResponse struct {
	Value string `json:"value"`
}

type AssetStack struct {
	ID             string `json:"id"`
	PrimaryAssetID string `json:"primaryAssetId"`
}

type StackResponse struct {
	ID             string          `json:"id"`
	PrimaryAssetID string          `json:"primaryAssetId"`
	Assets         []AssetResponse `json:"assets"`
}

type UpdateStackRequest struct {
	PrimaryAssetID string `json:"primaryAssetId"`
}

type ExifInfo struct {
	Latitude         *float64 `json:"latitude"`
	Longitude        *float64 `json:"longitude"`
	Description      *string  `json:"description"`
	Rating           *int     `json:"rating"`
	City             *string  `json:"city"`
	State            *string  `json:"state"`
	Country          *string  `json:"country"`
	DateTimeOriginal *string  `json:"dateTimeOriginal"`
	TimeZone         *string  `json:"timeZone"`
	Make             *string  `json:"make"`
	Model            *string  `json:"model"`
	LensModel        *string  `json:"lensModel"`
}

type SearchMetadataRequest struct {
	Page       int      `json:"page"`
	Size       int      `json:"size"`
	WithExif   bool     `json:"withExif"`
	WithPeople bool     `json:"withPeople,omitempty"`
	AlbumIDs   []string `json:"albumIds,omitempty"`
	Visibility string   `json:"visibility,omitempty"`
}

type ServerAbout struct {
	Version string `json:"version"`
}

type ServerFeatures struct {
	Trash bool `json:"trash"`
}

type SearchMetadataResponse struct {
	Assets SearchAssets `json:"assets"`
}

type SearchAssets struct {
	Items    []AssetResponse `json:"items"`
	NextPage *string         `json:"nextPage"`
}

type AlbumResponse struct {
	ID         string          `json:"id"`
	AlbumName  string          `json:"albumName"`
	AssetCount int             `json:"assetCount"`
	Assets     []AssetResponse `json:"assets"`
}

type CopyAssetsRequest struct {
	SourceID    string `json:"sourceId"`
	TargetID    string `json:"targetId"`
	Albums      bool   `json:"albums"`
	Favorite    bool   `json:"favorite"`
	SharedLinks bool   `json:"sharedLinks"`
	Sidecar     bool   `json:"sidecar"`
	Stack       bool   `json:"stack"`
}

type DeleteAssetsRequest struct {
	IDs   []string `json:"ids"`
	Force bool     `json:"force"`
}

type UpdateAssetsRequest struct {
	IDs        []string `json:"ids"`
	Visibility string   `json:"visibility,omitempty"`
}

type UploadResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type PathMapping struct {
	Server string
	Local  string
}

func (c *Config) MapServerPath(serverPath string) (string, string, bool) {
	serverPath = path.Clean(serverPath)
	var best PathMapping
	found := false
	for _, mapping := range c.InPlaceMaps {
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

func (c *Config) BakesEdits(asset AssetResponse) bool {
	return c.ApplyEdits && asset.IsEdited
}

// WillReplace reports whether this run mutates server assets; dry-run and
// export are read-only.
func (c *Config) WillReplace() bool {
	return !c.DryRun && c.ExportDir == ""
}

type ProcessResult struct {
	AssetID     string
	Status      ResultStatus
	Message     string
	NewID       string
	DuplicateID string
	Cancelled   bool
	StopReason  string
	ExifMatched bool
}

type ResultStatus int

const (
	StatusSuccess ResultStatus = iota
	StatusSkipped
	StatusFailed
)

func (s ResultStatus) String() string {
	switch s {
	case StatusSuccess:
		return "success"
	case StatusSkipped:
		return "skipped"
	case StatusFailed:
		return "failed"
	default:
		return "unknown"
	}
}

type Config struct {
	URL          string
	APIKey       string
	ImmichAPI    string
	Workers      int
	DryRun       bool
	ExportDir    string
	Yes          bool
	VerifyUpload bool
	Faces        bool
	ApplyEdits   bool
	InPlaceMaps  []PathMapping

	Rename        bool
	RenamePattern string

	ListAlbums            bool
	RepairStacks          bool
	ResolveDuplicate      bool
	IncludeNoAlbum        bool
	AssetIDs              []string
	AlbumIDs              []string
	ExportAlbumIDsByAsset map[string][]string
	AllAlbums             bool
	All                   bool
	Force                 bool
	TmpDir                string
}

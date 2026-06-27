package model

import "time"

type AssetResponse struct {
	ID               string    `json:"id"`
	DeviceAssetID    string    `json:"deviceAssetId"`
	DeviceID         string    `json:"deviceId"`
	OriginalFileName string    `json:"originalFileName"`
	OriginalMimeType string    `json:"originalMimeType"`
	Checksum         string    `json:"checksum"`
	FileCreatedAt    time.Time `json:"fileCreatedAt"`
	FileModifiedAt   time.Time `json:"fileModifiedAt"`
	IsFavorite       bool      `json:"isFavorite"`
	IsArchived       bool      `json:"isArchived"`
	Visibility       string    `json:"visibility"`
	ExifInfo         *ExifInfo `json:"exifInfo"`
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
	Make             *string  `json:"make"`
	Model            *string  `json:"model"`
	LensModel        *string  `json:"lensModel"`
}

type SearchMetadataRequest struct {
	Page     int      `json:"page"`
	Size     int      `json:"size"`
	WithExif bool     `json:"withExif,omitempty"`
	AlbumIDs []string `json:"albumIds,omitempty"`
}

type ServerAbout struct {
	Version string `json:"version"`
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

type ProcessResult struct {
	AssetID     string
	Status      ResultStatus
	Message     string
	NewID       string
	DuplicateID string
	Cancelled   bool
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

	TUI                   bool
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

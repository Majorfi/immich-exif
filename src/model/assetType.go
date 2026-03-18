package model

import (
	"path/filepath"
	"strings"
)

func IsVideoAsset(asset AssetResponse) bool {
	mimeType := AssetMimeType(asset)
	if strings.HasPrefix(mimeType, "video/") {
		return true
	}

	extension := AssetExtension(asset)
	switch extension {
	case ".mp4", ".mov", ".avi", ".mkv", ".wmv", ".m4v", ".3gp", ".3g2", ".mts", ".m2ts", ".webm", ".mpg", ".mpeg":
		return true
	}
	return false
}

func SupportsVideoMetadataEmbedding(asset AssetResponse) bool {
	if !IsVideoAsset(asset) {
		return false
	}

	switch AssetMimeType(asset) {
	case "video/mp4", "video/quicktime", "video/x-m4v":
		return true
	}

	switch AssetExtension(asset) {
	case ".mp4", ".mov", ".m4v":
		return true
	}

	return false
}

func IsUnsupportedVideoAsset(asset AssetResponse) bool {
	return IsVideoAsset(asset) && !SupportsVideoMetadataEmbedding(asset)
}

func AssetMimeType(asset AssetResponse) string {
	return strings.ToLower(strings.TrimSpace(asset.OriginalMimeType))
}

func AssetExtension(asset AssetResponse) string {
	return strings.ToLower(filepath.Ext(strings.TrimSpace(asset.OriginalFileName)))
}

package model

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// DecodeSHA1Checksum parses an Immich asset checksum, which the API may return
// either base64- or hex-encoded, into its raw SHA-1 bytes.
func DecodeSHA1Checksum(checksum string) ([]byte, error) {
	checksum = strings.TrimSpace(checksum)
	if b, err := base64.StdEncoding.DecodeString(checksum); err == nil && len(b) == sha1.Size {
		return b, nil
	}
	if b, err := hex.DecodeString(checksum); err == nil && len(b) == sha1.Size {
		return b, nil
	}
	return nil, fmt.Errorf("unrecognized checksum format")
}

func IsPathChecksum(asset AssetResponse) bool {
	stored, err := DecodeSHA1Checksum(asset.Checksum)
	if err != nil || asset.OriginalPath == "" {
		return false
	}
	pathSum := sha1.Sum([]byte("path:" + asset.OriginalPath))
	return bytes.Equal(stored, pathSum[:])
}

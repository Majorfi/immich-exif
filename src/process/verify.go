package process

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"io"
	"os"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/model"
)

// verifyUploadedChecksum re-fetches the uploaded asset and confirms its stored
// checksum matches the bytes of the local file we sent. It returns an error if
// the asset cannot be verified, so the caller can refuse to delete the original.
func verifyUploadedChecksum(client *api.ImmichClient, filePath, targetID string) error {
	want, err := fileChecksumSHA1(filePath)
	if err != nil {
		return fmt.Errorf("compute local checksum: %w", err)
	}
	newAsset, err := client.GetAsset(targetID)
	if err != nil {
		return fmt.Errorf("fetch uploaded asset: %w", err)
	}
	got, err := decodeChecksum(newAsset.Checksum)
	if err != nil {
		return fmt.Errorf("decode server checksum %q: %w", newAsset.Checksum, err)
	}
	if !bytes.Equal(want, got) {
		return fmt.Errorf("checksum mismatch for uploaded asset %s", model.ShortID(targetID))
	}
	return nil
}

func fileChecksumSHA1(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

func decodeChecksum(checksum string) ([]byte, error) {
	return model.DecodeSHA1Checksum(checksum)
}

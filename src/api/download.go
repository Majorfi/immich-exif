package api

import (
	"context"
	"crypto/sha1"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/majorfi/immich-exif/model"
)

func (c *ImmichClient) download(assetID, path string, destFor func(*http.Response) (string, error), onProgress func(percent int)) (sum []byte, err error) {
	req, err := c.newRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	watchdog := newProgressWatchdog(cancel)
	defer watchdog.Stop()
	resp, err := c.doRequest(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	destPath, err := destFor(resp)
	if err != nil {
		return nil, err
	}
	f, err := os.Create(destPath)
	if err != nil {
		return nil, fmt.Errorf("create file: %w", err)
	}
	defer func() {
		closeErr := f.Close()
		if closeErr != nil && err == nil {
			err = fmt.Errorf("close file: %w", closeErr)
		}
		if err != nil {
			_ = os.Remove(destPath)
		}
	}()

	hasher := sha1.New()
	dest := io.MultiWriter(f, hasher)
	if onProgress != nil && resp.ContentLength > 0 {
		dest = io.MultiWriter(f, hasher, &downloadProgressWriter{total: resp.ContentLength, onProgress: onProgress})
	}
	written, err := io.Copy(dest, watchdogReader{reader: resp.Body, watchdog: watchdog})
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("download stalled: no progress for %v", stallTimeout)
		}
		return nil, fmt.Errorf("write file: %w", err)
	}

	if resp.ContentLength >= 0 && written != resp.ContentLength {
		return nil, fmt.Errorf("download truncated for asset %s: wrote %d bytes, expected %d", model.ShortID(assetID), written, resp.ContentLength)
	}
	return hasher.Sum(nil), nil
}

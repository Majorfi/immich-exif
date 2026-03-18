package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/majorfi/immich-exif/exif"
	"github.com/majorfi/immich-exif/model"
)

type AssetSelectionStats struct {
	NoWritableMetadataSkipped int
	UnsupportedVideoSkipped   int
	StateSkipped              int
}

func (c *ImmichClient) SearchAssets(page, size int) (*model.SearchAssets, error) {
	body := model.SearchMetadataRequest{
		Page:     page,
		Size:     size,
		WithExif: true,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := c.newRequest(http.MethodPost, "/search/metadata", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	var resp model.SearchMetadataResponse
	if err := c.doJSON(req, &resp); err != nil {
		return nil, err
	}
	return &resp.Assets, nil
}

func (c *ImmichClient) ListAllAssetIDs(shouldSkip func(model.AssetResponse) bool) ([]string, AssetSelectionStats, error) {
	var allIDs []string
	stats := AssetSelectionStats{}
	page := 1
	pageSize := 1000

	for {
		result, err := c.SearchAssets(page, pageSize)
		if err != nil {
			return nil, AssetSelectionStats{}, fmt.Errorf("search page %d: %w", page, err)
		}

		for _, asset := range result.Items {
			if model.IsUnsupportedVideoAsset(asset) {
				stats.UnsupportedVideoSkipped++
				continue
			}
			if !exif.HasAssetMetadataToEmbed(asset) {
				stats.NoWritableMetadataSkipped++
				continue
			}
			if shouldSkip != nil && shouldSkip(asset) {
				stats.StateSkipped++
				continue
			}
			allIDs = append(allIDs, asset.ID)
		}

		nextPage, done, err := parseNextPage(result.NextPage)
		if err != nil {
			return nil, AssetSelectionStats{}, fmt.Errorf("invalid nextPage: %w", err)
		}
		if done {
			break
		}
		if nextPage <= page {
			return nil, AssetSelectionStats{}, fmt.Errorf("invalid nextPage %d after page %d", nextPage, page)
		}
		page = nextPage
	}

	return allIDs, stats, nil
}

func parseNextPage(nextPage *string) (int, bool, error) {
	if nextPage == nil {
		return 0, true, nil
	}
	token := strings.TrimSpace(*nextPage)
	if token == "" || strings.EqualFold(token, "null") {
		return 0, true, nil
	}

	page, err := strconv.Atoi(token)
	if err != nil {
		return 0, false, fmt.Errorf("%q: %w", token, err)
	}
	return page, false, nil
}

func (c *ImmichClient) ListAlbumIDs() ([]string, error) {
	req, err := c.newRequest(http.MethodGet, "/albums", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	var albums []model.AlbumResponse
	if err := c.doJSON(req, &albums); err != nil {
		return nil, err
	}

	albumIDs := make([]string, 0, len(albums))
	for _, album := range albums {
		if strings.TrimSpace(album.ID) == "" {
			continue
		}
		albumIDs = append(albumIDs, album.ID)
	}
	return albumIDs, nil
}

func (c *ImmichClient) GetAlbumAssets(albumID string) ([]string, error) {
	req, err := c.newRequest(http.MethodGet, "/albums/"+albumID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	var album model.AlbumResponse
	if err := c.doJSON(req, &album); err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(album.Assets))
	for _, asset := range album.Assets {
		ids = append(ids, asset.ID)
	}
	return ids, nil
}

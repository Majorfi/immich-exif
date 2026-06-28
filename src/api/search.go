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

func (c *ImmichClient) searchAssetsPage(page, size int, albumIDs []string, withExif bool, visibility string) (*model.SearchAssets, error) {
	body := model.SearchMetadataRequest{
		Page:       page,
		Size:       size,
		WithExif:   withExif,
		AlbumIDs:   albumIDs,
		Visibility: visibility,
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

func (c *ImmichClient) forEachSearchPage(albumIDs []string, withExif bool, visibility string, handle func([]model.AssetResponse)) error {
	page := 1
	pageSize := 1000

	for {
		result, err := c.searchAssetsPage(page, pageSize, albumIDs, withExif, visibility)
		if err != nil {
			return fmt.Errorf("search page %d: %w", page, err)
		}

		handle(result.Items)

		nextPage, done, err := parseNextPage(result.NextPage)
		if err != nil {
			return fmt.Errorf("invalid nextPage: %w", err)
		}
		if done {
			break
		}
		if nextPage <= page {
			return fmt.Errorf("invalid nextPage %d after page %d", nextPage, page)
		}
		page = nextPage
	}

	return nil
}

func (c *ImmichClient) ListAllAssetIDs(shouldSkip func(model.AssetResponse) bool) ([]string, AssetSelectionStats, error) {
	var allIDs []string
	stats := AssetSelectionStats{}

	err := c.forEachSearchPage(nil, true, "", func(items []model.AssetResponse) {
		for _, asset := range items {
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
	})
	if err != nil {
		return nil, AssetSelectionStats{}, err
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
	if strings.TrimSpace(albumID) == "" {
		return nil, fmt.Errorf("empty album ID")
	}
	if c.apiV3 {
		return c.searchAlbumAssetIDs(albumID)
	}

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
	// A newer server omits inline assets from the album response; if the album
	// reports assets but none were inlined, page them via the search endpoint.
	if len(ids) == 0 && album.AssetCount > 0 {
		return c.searchAlbumAssetIDs(albumID)
	}
	return ids, nil
}

// albumSearchVisibilities are the visibilities enumerated when paging an album
// via search/metadata. The endpoint filters by a single visibility (defaulting
// to timeline), so each must be queried to cover archived and hidden members.
// "locked" is excluded: it requires an elevated session.
var albumSearchVisibilities = []string{"timeline", "archive", "hidden"}

func (c *ImmichClient) searchAlbumAssetIDs(albumID string) ([]string, error) {
	var ids []string
	seen := map[string]bool{}

	for _, visibility := range albumSearchVisibilities {
		err := c.forEachSearchPage([]string{albumID}, false, visibility, func(items []model.AssetResponse) {
			for _, asset := range items {
				if seen[asset.ID] {
					continue
				}
				seen[asset.ID] = true
				ids = append(ids, asset.ID)
			}
		})
		if err != nil {
			return nil, fmt.Errorf("search album %s (visibility %s): %w", albumID, visibility, err)
		}
	}

	return ids, nil
}

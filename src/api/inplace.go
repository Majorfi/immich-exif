package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
)

func (c *ImmichClient) GetSidecarPaths(assetID string) ([]string, error) {
	req, err := c.newRequest(http.MethodGet, "/asset-files?type=sidecar&assetId="+url.QueryEscape(assetID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	var files []struct {
		Path string `json:"path"`
	}
	if err := c.doJSON(req, &files); err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	return paths, nil
}

func (c *ImmichClient) RefreshMetadata(assetID string) error {
	body, err := json.Marshal(struct {
		AssetIDs []string `json:"assetIds"`
		Name     string   `json:"name"`
	}{AssetIDs: []string{assetID}, Name: "refresh-metadata"})
	if err != nil {
		return err
	}
	req, err := c.newRequest(http.MethodPost, "/assets/jobs", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.doRequest(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return nil
}

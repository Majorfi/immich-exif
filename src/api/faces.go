package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"github.com/majorfi/immich-exif/model"
)

// GetAssetFaces returns the detected face boxes (with their assigned person,
// when any) for one asset. The asset detail response stopped inlining face
// boxes on v3, so this dedicated endpoint is used on every contract.
func (c *ImmichClient) GetAssetFaces(assetID string) ([]model.AssetFaceResponse, error) {
	req, err := c.newRequest(http.MethodGet, "/faces?id="+url.QueryEscape(assetID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	var faces []model.AssetFaceResponse
	if err := c.doJSON(req, &faces); err != nil {
		return nil, err
	}
	return faces, nil
}

// CreateFace links a person to an asset at a pixel box (POST /faces), the same
// endpoint the web UI uses for manual face tagging. It preserves a named person
// on a re-uploaded asset when metadata regions cannot — notably videos, where
// Immich re-detects boxes on the new thumbnail but loses the person link.
func (c *ImmichClient) CreateFace(face model.CreateFaceRequest) error {
	jsonBody, err := json.Marshal(face)
	if err != nil {
		return err
	}
	req, err := c.newRequest(http.MethodPost, "/faces", bytes.NewReader(jsonBody))
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

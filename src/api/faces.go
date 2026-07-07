package api

import (
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

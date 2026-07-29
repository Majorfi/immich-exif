package model

import "testing"

func TestHasFaceRegionsToEmbedAllowsSupportedVideos(t *testing.T) {
	asset := AssetResponse{
		OriginalMimeType: "video/mp4",
		People:           []PersonResponse{{ID: "p1", Name: "Alice"}},
	}
	if !HasFaceRegionsToEmbed(asset) {
		t.Fatal("mp4 videos with named people are region-embeddable")
	}
}

func TestHasFaceRegionsToEmbedExcludesUnsupportedVideos(t *testing.T) {
	asset := AssetResponse{
		OriginalMimeType: "video/x-matroska",
		OriginalFileName: "clip.mkv",
		People:           []PersonResponse{{ID: "p1", Name: "Alice"}},
	}
	if HasFaceRegionsToEmbed(asset) {
		t.Fatal("containers exiftool cannot write must be excluded even with named people")
	}
}

func TestHasFaceRegionsToEmbedRequiresNamedVisiblePerson(t *testing.T) {
	asset := AssetResponse{OriginalMimeType: "image/jpeg"}
	if HasFaceRegionsToEmbed(asset) {
		t.Fatal("no people means nothing to embed")
	}
	asset.People = []PersonResponse{
		{ID: "p1", Name: "   "},
		{ID: "p2", Name: "Bob", IsHidden: true},
	}
	if HasFaceRegionsToEmbed(asset) {
		t.Fatal("blank-named and hidden people must not count")
	}
	asset.People = append(asset.People, PersonResponse{ID: "p3", Name: "Carol"})
	if !HasFaceRegionsToEmbed(asset) {
		t.Fatal("one visible, named person makes the asset embeddable")
	}
}

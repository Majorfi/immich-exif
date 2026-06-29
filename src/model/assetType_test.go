package model

import "testing"

func TestIsVideoAsset(t *testing.T) {
	testCases := []struct {
		name  string
		asset AssetResponse
		want  bool
	}{
		{
			name: "video mime type",
			asset: AssetResponse{
				OriginalFileName: "image.jpg",
				OriginalMimeType: "video/mp4",
			},
			want: true,
		},
		{
			name: "video extension fallback",
			asset: AssetResponse{
				OriginalFileName: "clip.MP4",
				OriginalMimeType: "",
			},
			want: true,
		},
		{
			name: "image asset",
			asset: AssetResponse{
				OriginalFileName: "photo.jpg",
				OriginalMimeType: "image/jpeg",
			},
			want: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := IsVideoAsset(testCase.asset)
			if got != testCase.want {
				t.Fatalf("expected %v, got %v", testCase.want, got)
			}
		})
	}
}

func TestSupportsVideoMetadataEmbedding(t *testing.T) {
	testCases := []struct {
		name  string
		asset AssetResponse
		want  bool
	}{
		{
			name: "mp4 supported by mime type",
			asset: AssetResponse{
				OriginalFileName: "clip.bin",
				OriginalMimeType: "video/mp4",
			},
			want: true,
		},
		{
			name: "mov supported by extension fallback",
			asset: AssetResponse{
				OriginalFileName: "clip.MOV",
			},
			want: true,
		},
		{
			name: "webm not supported",
			asset: AssetResponse{
				OriginalFileName: "clip.webm",
				OriginalMimeType: "video/webm",
			},
			want: false,
		},
		{
			name: "image not supported",
			asset: AssetResponse{
				OriginalFileName: "photo.jpg",
				OriginalMimeType: "image/jpeg",
			},
			want: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := SupportsVideoMetadataEmbedding(testCase.asset)
			if got != testCase.want {
				t.Fatalf("expected %v, got %v", testCase.want, got)
			}
		})
	}
}

func TestIsUnsupportedVideoAsset(t *testing.T) {
	cases := []struct {
		name  string
		asset AssetResponse
		want  bool
	}{
		{name: "supported mp4", asset: AssetResponse{OriginalFileName: "a.mp4", OriginalMimeType: "video/mp4"}, want: false},
		{name: "unsupported webm", asset: AssetResponse{OriginalFileName: "c.webm", OriginalMimeType: "video/webm"}, want: true},
		{name: "photo", asset: AssetResponse{OriginalFileName: "p.jpg", OriginalMimeType: "image/jpeg"}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsUnsupportedVideoAsset(tc.asset); got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

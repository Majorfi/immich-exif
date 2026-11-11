package model

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"testing"
)

func TestDecodeSHA1Checksum(t *testing.T) {
	raw := sha1.Sum([]byte("hello"))
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "base64", input: base64.StdEncoding.EncodeToString(raw[:])},
		{name: "hex", input: hex.EncodeToString(raw[:])},
		{name: "trimmed", input: "  " + hex.EncodeToString(raw[:]) + "  "},
		{name: "empty", input: "", wantErr: true},
		{name: "garbage", input: "!!notbase64!!", wantErr: true},
		{name: "wrong length", input: base64.StdEncoding.EncodeToString([]byte("short")), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DecodeSHA1Checksum(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(got) != string(raw[:]) {
				t.Fatal("decoded bytes mismatch")
			}
		})
	}
}

func TestIsPathChecksumRecognizesExternalLibraryHash(t *testing.T) {
	asset := AssetResponse{OriginalPath: "/mnt/external/ext1.jpg", Checksum: "zRGfxxWk2xWu6fDXr8dMonW6tuY="}
	if !IsPathChecksum(asset) {
		t.Fatal("the hash of path:<originalPath> must be recognized (value captured from Immich 3.3.6)")
	}
	asset.OriginalPath = "/mnt/external/other.jpg"
	if IsPathChecksum(asset) {
		t.Fatal("a checksum of another path must not match")
	}
	if IsPathChecksum(AssetResponse{Checksum: "zRGfxxWk2xWu6fDXr8dMonW6tuY="}) {
		t.Fatal("without a path the checksum cannot be a path hash")
	}
}

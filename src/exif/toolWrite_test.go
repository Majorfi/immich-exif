package exif

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

const tinyJPEGBase64 = "/9j/4AAQSkZJRgABAQAAAQABAAD/2wCEAAkGBxAQEBAQEA8PEA8PDw8PDw8PDw8PDw8QFREWFhURFRUYHSggGBolGxUVITEhJSkrLi4uFx8zODMsNygtLisBCgoKDg0OGhAQGi0fHyUtLS0tLS0tLS0tLS0tLS0tLS0tLS0tLS0tLS0tLS0tLS0tLS0tLS0tLS0tLS0tLS0tLf/AABEIAAEAAQMBEQACEQEDEQH/xAAXAAEBAQEAAAAAAAAAAAAAAAAAAQID/8QAFBABAAAAAAAAAAAAAAAAAAAAAP/aAAwDAQACEAMQAAAB6AAAAP/EABQQAQAAAAAAAAAAAAAAAAAAADD/2gAIAQEAAT8Af//EABQRAQAAAAAAAAAAAAAAAAAAADD/2gAIAQIBAT8Af//EABQRAQAAAAAAAAAAAAAAAAAAADD/2gAIAQMBAT8Af//Z"

func TestWriteExifTagsHandlesMultilineValues(t *testing.T) {
	if err := CheckExiftool(); err != nil {
		t.Skipf("exiftool unavailable: %v", err)
	}

	filePath := filepath.Join(t.TempDir(), "tiny.jpg")
	data, err := base64.StdEncoding.DecodeString(tinyJPEGBase64)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if err := WriteExifTags(filePath, []string{"-ImageDescription=Line one\nLine two"}); err != nil {
		t.Fatalf("write exif tags: %v", err)
	}

	values, err := ReadExifTags(filePath)
	if err != nil {
		t.Fatalf("read exif tags: %v", err)
	}

	description, ok := values["ImageDescription"].(string)
	if !ok {
		t.Fatalf("expected string description, got %T", values["ImageDescription"])
	}
	if description != "Line one\nLine two" {
		t.Fatalf("expected multiline description, got %q", description)
	}
}

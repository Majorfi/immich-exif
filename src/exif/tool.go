package exif

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

type ExifTagMap map[string]any

var ReadExifTagsFn = ReadExifTags
var WriteExifTagsFn = WriteExifTags
var CheckExiftoolFn = CheckExiftool

func CheckExiftool() error {
	_, err := exec.LookPath("exiftool")
	if err != nil {
		return fmt.Errorf("exiftool not found in PATH: %w", err)
	}
	return nil
}

func ReadExifTags(filePath string) (ExifTagMap, error) {
	baseTags, err := readExifTagsWithArgs(filePath, []string{"-json", "-n"})
	if err != nil {
		return nil, err
	}

	supplementalTags, err := readExifTagsWithArgs(filePath, []string{
		"-json", "-n", "-a", "-G1",
		"-IPTC:City",
		"-XMP-photoshop:City",
		"-IPTC:Province-State",
		"-XMP-photoshop:State",
		"-IPTC:Country-PrimaryLocationName",
		"-XMP-photoshop:Country",
		"-XMP-dc:Description",
		"-IPTC:Caption-Abstract",
		"-XMP-xmp:Rating",
		"-XMP-exif:GPSLatitude",
		"-XMP-exif:GPSLongitude",
		"-XMP-exif:DateTimeOriginal",
		"-XMP-xmp:CreateDate",
	})
	if err == nil {
		for key, value := range supplementalTags {
			baseTags[key] = value
		}
	}

	return baseTags, nil
}

func readExifTagsWithArgs(filePath string, exifArgs []string) (ExifTagMap, error) {
	cmdArgs := append([]string{}, exifArgs...)
	cmdArgs = append(cmdArgs, "--", filePath)
	cmd := exec.Command("exiftool", cmdArgs...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("exiftool read: %w", err)
	}

	var results []ExifTagMap
	if err := json.Unmarshal(out, &results); err != nil {
		return nil, fmt.Errorf("parse exiftool output: %w", err)
	}

	if len(results) == 0 {
		return ExifTagMap{}, nil
	}
	return results[0], nil
}

func WriteExifTags(filePath string, args []string) error {
	cmdArgs := append([]string{}, args...)
	cmdArgs = append(cmdArgs, "-overwrite_original", "--", filePath)
	cmd := exec.Command("exiftool", cmdArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("exiftool write: %w: %s", err, string(output))
	}
	return nil
}

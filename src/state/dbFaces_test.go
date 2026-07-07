package state

import (
	"strings"
	"testing"

	"github.com/majorfi/immich-exif/model"
)

func TestSnapshotAssetIncludesNamedPeople(t *testing.T) {
	asset := model.AssetResponse{
		ID: "a1",
		People: []model.PersonResponse{
			{ID: "p2", Name: "Zoe"},
			{ID: "p1", Name: "Alice"},
			{ID: "p3", Name: ""},
			{ID: "p4", Name: "Hidden", IsHidden: true},
		},
	}
	snap, err := SnapshotAsset(asset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(snap, `"peopleNames":["Alice","Zoe"]`) {
		t.Fatalf("expected sorted visible named people in snapshot, got %s", snap)
	}

	asset.People[1].Name = "Alicia"
	renamed, err := SnapshotAsset(asset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if renamed == snap {
		t.Fatal("renaming a person must change the snapshot so the asset re-processes")
	}
}

func TestSnapshotAssetWithoutPeopleKeepsLegacyShape(t *testing.T) {
	snap, err := SnapshotAsset(model.AssetResponse{ID: "a1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(snap, "peopleNames") {
		t.Fatalf("assets without people must keep their pre-faces snapshot hash, got %s", snap)
	}
}

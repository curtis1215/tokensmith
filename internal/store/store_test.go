package store

import (
	"path/filepath"
	"testing"

	"tokensmith/internal/model"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "save.json")
	var s model.GameState
	s.Resources.Cash = 12345
	s.Resources.RnD = 6789
	s.Models = []model.Model{{Gen: 2, Online: true, Users: 1000, Price: 12}}
	s.Prestige.Patents = 3
	s.HiredStars = []string{"aria-chen"}
	if err := Save(path, s); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, ok, err := Load(path)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got.Resources.Cash != 12345 || got.Resources.RnD != 6789 {
		t.Errorf("resources not restored: %+v", got.Resources)
	}
	if len(got.Models) != 1 || got.Models[0].Users != 1000 {
		t.Errorf("models not restored: %+v", got.Models)
	}
	if got.Prestige.Patents != 3 || len(got.HiredStars) != 1 {
		t.Errorf("prestige/stars not restored: %+v %+v", got.Prestige, got.HiredStars)
	}
}

func TestLoadMissing(t *testing.T) {
	_, ok, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil || ok {
		t.Fatalf("missing file: ok=%v err=%v, want false/nil", ok, err)
	}
}

package genre_test

import (
	"testing"

	storyforge "storyforge"
	"storyforge/internal/genre"
)

func TestLoadFromFS_AllGenres(t *testing.T) {
	fsys, err := storyforge.GenresFS()
	if err != nil {
		t.Fatalf("genres fs: %v", err)
	}
	reg, err := genre.LoadFromFS(fsys)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// Check ZH genres
	zhGenres := []string{"xuanhuan", "xianxia", "urban", "horror", "other"}
	for _, id := range zhGenres {
		g, ok := reg.Get("zh", id)
		if !ok {
			t.Errorf("zh/%s not found", id)
			continue
		}
		if len(g.FatigueWords) == 0 {
			t.Errorf("zh/%s: fatigueWords is empty", id)
		}
		if len(g.Rules) == 0 {
			t.Errorf("zh/%s: rules is empty", id)
		}
	}

	// Check EN genres
	enGenres := []string{"litrpg", "progression-fantasy", "isekai", "cultivation",
		"system-apocalypse", "dungeon-core", "romantasy", "sci-fi", "tower-climber", "cozy"}
	for _, id := range enGenres {
		g, ok := reg.Get("en", id)
		if !ok {
			t.Errorf("en/%s not found", id)
			continue
		}
		if len(g.FatigueWords) == 0 {
			t.Errorf("en/%s: fatigueWords is empty", id)
		}
		if len(g.Rules) == 0 {
			t.Errorf("en/%s: rules is empty", id)
		}
	}
}

func TestLoadFromFS_NumericalSystem(t *testing.T) {
	fsys, err := storyforge.GenresFS()
	if err != nil {
		t.Fatal(err)
	}
	reg, _ := genre.LoadFromFS(fsys)

	litrpg, ok := reg.Get("en", "litrpg")
	if !ok {
		t.Fatal("litrpg not found")
	}
	if !litrpg.NumericalSystem {
		t.Error("litrpg should have numericalSystem=true")
	}
	if !litrpg.PowerScaling {
		t.Error("litrpg should have powerScaling=true")
	}

	cozy, ok := reg.Get("en", "cozy")
	if !ok {
		t.Fatal("cozy not found")
	}
	if cozy.NumericalSystem {
		t.Error("cozy should have numericalSystem=false")
	}
}

func TestLoadFromFS_AuditDimensions(t *testing.T) {
	fsys, err := storyforge.GenresFS()
	if err != nil {
		t.Fatal(err)
	}
	reg, _ := genre.LoadFromFS(fsys)

	litrpg, _ := reg.Get("en", "litrpg")
	if len(litrpg.AuditDimensions) == 0 {
		t.Error("litrpg should have audit dimensions")
	}

	found := false
	for _, d := range litrpg.AuditDimensions {
		if d == "numerical" {
			found = true
		}
	}
	if !found {
		t.Error("litrpg should have 'numerical' audit dimension")
	}
}

func TestRegistry_List(t *testing.T) {
	fsys, err := storyforge.GenresFS()
	if err != nil {
		t.Fatal(err)
	}
	reg, _ := genre.LoadFromFS(fsys)

	zhList := reg.List("zh")
	if len(zhList) < 5 {
		t.Errorf("expected at least 5 zh genres, got %d", len(zhList))
	}

	enList := reg.List("en")
	if len(enList) < 10 {
		t.Errorf("expected at least 10 en genres, got %d", len(enList))
	}
}

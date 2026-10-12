package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMaterializeRepositoryOptionsSparse(t *testing.T) {
	origin := createMaterializeOrigin(t)
	seed := filepath.Join(t.TempDir(), "seed")
	runMaterializeTestGit(t, filepath.Dir(seed), "clone", origin, seed)
	for _, dir := range []string{"app", "other"} {
		if err := os.Mkdir(filepath.Join(seed, dir), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(seed, dir, "file.txt"), []byte(dir), 0600); err != nil {
			t.Fatal(err)
		}
	}
	runMaterializeTestGit(t, seed, "add", ".")
	runMaterializeTestGit(t, seed, "-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "commit", "-m", "folders")
	runMaterializeTestGit(t, seed, "push", "origin", "main")
	var req MaterializeRepositoryRequest
	if err := json.Unmarshal([]byte(`{"checkout_options":{"version":1,"download_mode":"standard","sparse_directories":["app"]}}`), &req); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "checkout")
	_, err := materializeRepositoryWithOptions(context.Background(), origin, destination, "main", "", nil, nil, req.CheckoutOptions)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destination, "app", "file.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := materializeRepository(context.Background(), origin, destination, "main", ""); err == nil {
		t.Fatal("default request silently reused a sparse checkout")
	}

	if _, err := os.Stat(filepath.Join(destination, "other")); !os.IsNotExist(err) {
		t.Fatal("excluded directory downloaded")
	}
}

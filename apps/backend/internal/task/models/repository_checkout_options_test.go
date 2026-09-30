package models

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestRepositoryCheckoutOptionsAPIView(t *testing.T) {
	metadata := map[string]interface{}{}
	want := &RepositoryCheckoutOptions{Version: 1, DownloadMode: DownloadOnDemand, SparseDirectories: []string{"app"}}
	if err := PutRepositoryCheckoutOptions(metadata, want); err != nil {
		t.Fatal(err)
	}
	task := &Task{Repositories: []*TaskRepository{{Metadata: metadata}}}
	if got := task.ToAPI().Repositories[0].CheckoutOptions; !reflect.DeepEqual(got, want) {
		t.Fatalf("API options = %+v, want %+v", got, want)
	}
}

func TestRepositoryCheckoutOptionsNormalization(t *testing.T) {
	for _, directory := range []string{"", "/tmp", "../a", "a/../b", "a//b", "a/./b", "a/", "a\\b", "C:/tmp", "a\nb", "a\x00b", "app/*", strings.Repeat("x", 4097)} {
		t.Run(directory, func(t *testing.T) {
			_, err := NormalizeRepositoryCheckoutOptions(&RepositoryCheckoutOptions{Version: 1, DownloadMode: DownloadStandard, SparseDirectories: []string{directory}})
			if err == nil {
				t.Fatal("invalid directory accepted")
			}
		})
	}
	want := &RepositoryCheckoutOptions{Version: 1, DownloadMode: DownloadOnDemand, SparseDirectories: []string{"app one", "app two"}}
	in := &RepositoryCheckoutOptions{Version: 1, DownloadMode: DownloadOnDemand, SparseDirectories: []string{"app one", "app two", "app one"}}
	got, err := NormalizeRepositoryCheckoutOptions(in)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized options = %+v", got)
	}
	in.SparseDirectories[0] = "mutated"
	if got.SparseDirectories[0] != "app one" {
		t.Fatal("normalization shares input storage")
	}
	encoded, err := json.Marshal(map[string]interface{}{RepositoryCheckoutOptionsKey: got})
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]interface{}
	if err := json.Unmarshal(encoded, &stored); err != nil {
		t.Fatal(err)
	}
	decoded, err := GetRepositoryCheckoutOptions(stored)
	if err != nil || !reflect.DeepEqual(decoded, want) {
		t.Fatalf("stored options = %+v, err %v", decoded, err)
	}
}

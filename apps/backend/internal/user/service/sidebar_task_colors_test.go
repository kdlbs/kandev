package service

import (
	"context"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/user/models"
	"github.com/kandev/kandev/internal/user/store"
)

func TestUpdateUserSettingsSidebarTaskColorPatchSupportsNormalClearAndImport(t *testing.T) {
	red := "red"
	blue := "blue"
	green := "green"
	repo := newCASFakeRepo(&models.UserSettings{
		UserID: store.DefaultUserID,
		SidebarTaskColors: map[string]*string{
			"task-existing": &red,
			"task-cleared":  nil,
			"task-other":    &green,
		},
	})
	svc := newCASService(repo, nil)

	got, err := svc.UpdateUserSettings(context.Background(), &UpdateUserSettingsRequest{
		SidebarTaskColorPatch: &models.SidebarTaskColorPatch{
			Colors: map[string]*string{"task-existing": &blue},
		},
	})
	if err != nil {
		t.Fatalf("normal color patch: %v", err)
	}
	if got.SidebarTaskColors["task-existing"] == nil || *got.SidebarTaskColors["task-existing"] != "blue" {
		t.Fatalf("normal patch color = %#v, want blue", got.SidebarTaskColors["task-existing"])
	}
	if got.SidebarTaskColors["task-other"] == nil || *got.SidebarTaskColors["task-other"] != "green" {
		t.Fatalf("normal patch changed an unsupplied task: %#v", got.SidebarTaskColors["task-other"])
	}

	got, err = svc.UpdateUserSettings(context.Background(), &UpdateUserSettingsRequest{
		SidebarTaskColorPatch: &models.SidebarTaskColorPatch{
			Colors: map[string]*string{"task-existing": nil},
		},
	})
	if err != nil {
		t.Fatalf("clear color patch: %v", err)
	}
	if value, present := got.SidebarTaskColors["task-existing"]; !present || value != nil {
		t.Fatalf("clear patch = (%#v, %t), want (nil, true)", value, present)
	}

	got, err = svc.UpdateUserSettings(context.Background(), &UpdateUserSettingsRequest{
		SidebarTaskColorPatch: &models.SidebarTaskColorPatch{
			IfMissing: true,
			Colors: map[string]*string{
				"task-existing": &red,
				"task-cleared":  &blue,
				"task-new":      &green,
			},
		},
	})
	if err != nil {
		t.Fatalf("missing-only color patch: %v", err)
	}
	if value, present := got.SidebarTaskColors["task-existing"]; !present || value != nil {
		t.Fatalf("cleared existing color = (%#v, %t), want (nil, true)", value, present)
	}
	if value, present := got.SidebarTaskColors["task-cleared"]; !present || value != nil {
		t.Fatalf("existing tombstone = (%#v, %t), want (nil, true)", value, present)
	}
	if value := got.SidebarTaskColors["task-new"]; value == nil || *value != "green" {
		t.Fatalf("new imported color = %#v, want green", value)
	}
}

func TestUpdateUserSettingsSidebarTaskColorImportPreservesTombstones(t *testing.T) {
	red := "red"
	blue := "blue"
	repo := newCASFakeRepo(&models.UserSettings{
		UserID:            store.DefaultUserID,
		SidebarTaskColors: map[string]*string{"task-color": &red, "task-tombstone": nil},
	})
	svc := newCASService(repo, nil)

	got, err := svc.UpdateUserSettings(context.Background(), &UpdateUserSettingsRequest{
		SidebarTaskColorPatch: &models.SidebarTaskColorPatch{
			IfMissing: true,
			Colors: map[string]*string{
				"task-color":     &blue,
				"task-tombstone": &blue,
				"task-new":       &blue,
			},
		},
	})
	if err != nil {
		t.Fatalf("missing-only color patch: %v", err)
	}
	if value := got.SidebarTaskColors["task-color"]; value == nil || *value != "red" {
		t.Fatalf("existing color = %#v, want red", value)
	}
	if value, present := got.SidebarTaskColors["task-tombstone"]; !present || value != nil {
		t.Fatalf("existing tombstone = (%#v, %t), want (nil, true)", value, present)
	}
	if value := got.SidebarTaskColors["task-new"]; value == nil || *value != "blue" {
		t.Fatalf("missing color = %#v, want blue", value)
	}
}

func TestConcurrentSidebarTaskColorPatchesMergeDifferentTasks(t *testing.T) {
	red := "red"
	blue := "blue"
	repo := newCASFakeRepo(&models.UserSettings{UserID: store.DefaultUserID})
	repo.readDone = make(chan struct{}, 4)
	repo.blockUpsert = make(chan struct{})
	svc := newCASService(repo, nil)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := svc.UpdateUserSettings(context.Background(), &UpdateUserSettingsRequest{
			SidebarTaskColorPatch: &models.SidebarTaskColorPatch{
				Colors: map[string]*string{"task-red": &red},
			},
		})
		if err != nil {
			t.Errorf("red patch: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		_, err := svc.UpdateUserSettings(context.Background(), &UpdateUserSettingsRequest{
			SidebarTaskColorPatch: &models.SidebarTaskColorPatch{
				Colors: map[string]*string{"task-blue": &blue},
			},
		})
		if err != nil {
			t.Errorf("blue patch: %v", err)
		}
	}()

	<-repo.readDone
	<-repo.readDone
	close(repo.blockUpsert)
	wg.Wait()

	row := repo.snapshot()
	if value := row.SidebarTaskColors["task-red"]; value == nil || *value != "red" {
		t.Fatalf("red color = %#v, want red", value)
	}
	if value := row.SidebarTaskColors["task-blue"]; value == nil || *value != "blue" {
		t.Fatalf("blue color = %#v, want blue", value)
	}
	if row.Revision != 2 {
		t.Fatalf("revision = %d, want 2", row.Revision)
	}
	assertOneStaleConflictAndRetry(t, repo)
}

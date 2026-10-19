package backups

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/system/jobs"
	"github.com/stretchr/testify/require"
)

// @covers AC-SYSTEM-PAGE-SYSTEM-PAGE-001.9
func TestRestoreRejectsInvalidSnapshotBeforeDisruption(t *testing.T) {
	for _, damage := range []string{"empty", "truncated", "damaged-page", "non-sqlite"} {
		for _, legacy := range []bool{false, true} {
			name := damage + "/modern"
			if legacy {
				name = damage + "/legacy"
			}
			t.Run(name, func(t *testing.T) {
				svc, _ := newTestService(t)
				require.NoError(t, enableRestoreTestWAL(svc))
				receipt, err := svc.createSnapshot(context.Background(), true)
				require.NoError(t, err)
				snapshot := filepath.Join(svc.backupsDir(), receipt.Name)
				damageRestoreSnapshot(t, snapshot, damage)
				_, err = verifySnapshot(context.Background(), snapshot)
				if damage == "empty" {
					// SQLite treats an empty file as a valid empty database.
					require.NoError(t, err)
				} else {
					require.Error(t, err, "fixture must fail SQLite verification")
				}
				require.NoError(t, updateRestoreTestValue(svc, "current-unbacked-work"))
				paths := []string{svc.databasePath, svc.databasePath + "-wal", svc.databasePath + "-shm", snapshot}
				before := readRestoreTestFiles(t, paths)
				unavailable, quiesce, shutdown := 0, 0, 0
				svc.PersistenceUnavailable = func() { unavailable++ }
				svc.OrchestratorShutdown = func() { shutdown++ }
				if !legacy {
					svc.RestoreQuiesce = func() error { quiesce++; return nil }
				}
				id, err := svc.Restore(context.Background(), receipt.Name, RestoreConfirmToken)
				require.NoError(t, err)
				job := waitRestoreTerminal(t, svc, id)
				require.Equal(t, jobs.StateFailed, job.State, "damaged snapshot must fail before disrupting the working database")
				if damage == "empty" {
					require.Contains(t, job.Message, "restore snapshot is empty")
				}
				require.Zero(t, unavailable)
				require.Zero(t, quiesce)
				require.Zero(t, shutdown)
				require.Empty(t, job.Result)
				require.NoFileExists(t, svc.databasePath+".new")
				after := readRestoreTestFiles(t, paths)
				for i := range paths {
					require.True(t, bytes.Equal(before[i], after[i]), "file changed: %s", paths[i])
				}
				assertRestoreTestValue(t, svc, "current-unbacked-work")
				require.NoError(t, updateRestoreTestValue(svc, "work-after-rejection"))
				assertRestoreTestValue(t, svc, "work-after-rejection")
			})
		}
	}
}

func enableRestoreTestWAL(svc *Service) error {
	_, err := svc.pool.Writer().Exec("PRAGMA journal_mode=WAL; PRAGMA wal_autocheckpoint=0")
	return err
}

func updateRestoreTestValue(svc *Service, value string) error {
	_, err := svc.pool.Writer().Exec("UPDATE things SET name = ? WHERE id = '1'", value)
	return err
}

func assertRestoreTestValue(t *testing.T, svc *Service, want string) {
	t.Helper()
	var got string
	require.NoError(t, svc.pool.Writer().Get(&got, "SELECT name FROM things WHERE id = '1'"))
	require.Equal(t, want, got)
}

func damageRestoreSnapshot(t *testing.T, path, damage string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	switch damage {
	case "empty":
		data = nil
	case "truncated":
		data = data[:128]
	case "damaged-page":
		// The first b-tree page type follows the 100-byte database header.
		require.Greater(t, len(data), 100)
		data[100] = 0xff
	case "non-sqlite":
		data = []byte("this file is not a SQLite database")
	}
	require.NoError(t, os.WriteFile(path, data, 0600))
}

func readRestoreTestFiles(t *testing.T, paths []string) [][]byte {
	t.Helper()
	out := make([][]byte, len(paths))
	for i, path := range paths {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		out[i] = data
	}
	return out
}

func waitRestoreTerminal(t *testing.T, svc *Service, id string) *jobs.Job {
	t.Helper()
	var job *jobs.Job
	require.Eventually(t, func() bool {
		job = svc.jobs.Get(id)
		return job != nil && (job.State == jobs.StateFailed || job.State == jobs.StateSucceeded)
	}, 3*time.Second, 5*time.Millisecond)
	return job
}

func writeValidRestoreTestSnapshot(t *testing.T, destination string) []byte {
	t.Helper()
	source, _ := newTestService(t)
	receipt, err := source.createSnapshot(context.Background(), true)
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(source.backupsDir(), receipt.Name))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(destination, data, 0600))
	return data
}

// @covers AC-SYSTEM-PAGE-SYSTEM-PAGE-001.9
func TestRestoreValidSnapshotReplacesNewerData(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		name := "modern"
		if legacy {
			name = "legacy"
		}
		t.Run(name, func(t *testing.T) {
			svc, _ := newTestService(t)
			require.NoError(t, enableRestoreTestWAL(svc))
			receipt, err := svc.createSnapshot(context.Background(), true)
			require.NoError(t, err)
			require.NoError(t, updateRestoreTestValue(svc, "current-unbacked-work"))
			unavailable, quiesce, shutdown := 0, 0, 0
			svc.PersistenceUnavailable = func() { unavailable++ }
			svc.OrchestratorShutdown = func() { shutdown++ }
			if !legacy {
				svc.RestoreQuiesce = func() error { quiesce++; return nil }
			}
			id, err := svc.Restore(context.Background(), receipt.Name, RestoreConfirmToken)
			require.NoError(t, err)
			job := waitRestoreTerminal(t, svc, id)
			require.Equal(t, jobs.StateSucceeded, job.State, job.Message)
			require.Equal(t, true, job.Result["restart_required"])
			require.Equal(t, receipt.Name, job.Result["restored_from"])
			require.Equal(t, 1, unavailable)
			if legacy {
				require.Equal(t, 1, shutdown)
				require.Zero(t, quiesce)
			} else {
				require.Equal(t, 1, quiesce)
				require.Zero(t, shutdown)
			}
			require.Error(t, svc.pool.Writer().Ping())
			require.NoFileExists(t, svc.databasePath+".new")
			assertRestoredTestFileValue(t, svc.databasePath, "hello")
		})
	}
}

func assertRestoredTestFileValue(t *testing.T, path, want string) {
	t.Helper()
	reader, err := openSnapshotReader(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close() })
	var value string
	require.NoError(t, reader.Get(&value, "SELECT name FROM things WHERE id = '1'"))
	require.Equal(t, want, value)
}

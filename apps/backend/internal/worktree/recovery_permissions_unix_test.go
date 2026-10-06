//go:build unix

package worktree

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

const recoveryUmaskChild = "KANDEV_RECOVERY_UMASK_CHILD"

func TestRecoverySnapshotIgnoresProcessUmask(t *testing.T) {
	if os.Getenv(recoveryUmaskChild) == "1" {
		source := os.Getenv("KANDEV_RECOVERY_UMASK_SOURCE")
		snapshot := os.Getenv("KANDEV_RECOVERY_UMASK_SNAPSHOT")
		previous := syscall.Umask(0o022)
		defer syscall.Umask(previous)
		if err := snapshotCheckout(source, snapshot); err != nil {
			t.Fatalf("snapshotCheckout under umask 0022: %v", err)
		}
		assertRecoveryMode(t, filepath.Join(snapshot, "group-writable"), 0o775)
		return
	}

	source := t.TempDir()
	writeRecoveryModeFixture(t, filepath.Join(source, "group-writable"), 0o775)
	snapshot := filepath.Join(t.TempDir(), "snapshot")
	command := exec.Command(os.Args[0], "-test.run=^TestRecoverySnapshotIgnoresProcessUmask$")
	command.Env = append(os.Environ(),
		recoveryUmaskChild+"=1",
		"KANDEV_RECOVERY_UMASK_SOURCE="+source,
		"KANDEV_RECOVERY_UMASK_SNAPSHOT="+snapshot,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("child snapshot with umask 0022: %v\n%s", err, output)
	}
}

func TestRecoverySetIDPreservesRequiredIdentity(t *testing.T) {
	source := t.TempDir()
	filePath := filepath.Join(source, "setid-file")
	writeRecoveryUnixModeFixture(t, filePath, 0o6750)
	setGIDDir := filepath.Join(source, "setgid-directory")
	if err := os.Mkdir(setGIDDir, 0o700); err != nil {
		t.Fatalf("create setgid directory: %v", err)
	}
	stickyDir := filepath.Join(source, "sticky-directory")
	if err := os.Mkdir(stickyDir, 0o700); err != nil {
		t.Fatalf("create sticky directory: %v", err)
	}
	if err := chmodRecoveryUnixMode(filePath, 0o6750); err != nil {
		t.Fatalf("set set-ID file mode: %v", err)
	}
	if err := chmodRecoveryUnixMode(setGIDDir, 0o2750); err != nil {
		t.Fatalf("set setgid directory mode: %v", err)
	}
	if err := chmodRecoveryUnixMode(stickyDir, 0o1777); err != nil {
		t.Fatalf("set sticky directory mode: %v", err)
	}

	sourceInfo := statRecoveryEntry(t, filePath)
	fileUID, fileGID := recoveryTestIdentity(t, sourceInfo)
	_, directoryGID := recoveryTestIdentity(t, statRecoveryEntry(t, setGIDDir))
	snapshot := filepath.Join(t.TempDir(), "snapshot")
	if err := snapshotCheckout(source, snapshot); err != nil {
		t.Fatalf("snapshotCheckout: %v", err)
	}
	assertRecoveryModeAndIdentity(t, filepath.Join(snapshot, "setid-file"), 0o6750, fileUID, fileGID, true, true)
	assertRecoveryModeAndIdentity(t, filepath.Join(snapshot, "setgid-directory"), 0o2750, 0, directoryGID, false, true)
	assertRecoveryModeAndIdentity(t, filepath.Join(snapshot, "sticky-directory"), 0o1777, 0, 0, false, false)

	replacement := t.TempDir()
	if err := copySnapshotEntries(snapshot, replacement); err != nil {
		t.Fatalf("copySnapshotEntries: %v", err)
	}
	assertRecoveryModeAndIdentity(t, filepath.Join(replacement, "setid-file"), 0o6750, fileUID, fileGID, true, true)
	assertRecoveryModeAndIdentity(t, filepath.Join(replacement, "setgid-directory"), 0o2750, 0, directoryGID, false, true)
	assertRecoveryModeAndIdentity(t, filepath.Join(replacement, "sticky-directory"), 0o1777, 0, 0, false, false)
}

func TestRecoverySetIDPreservesDifferentRequiredUID(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires permission to create a source file with a different owner")
	}
	source := t.TempDir()
	filePath := filepath.Join(source, "setuid-file")
	writeRecoveryUnixModeFixture(t, filePath, 0o4755)
	if err := os.Chown(filePath, 65534, -1); err != nil {
		t.Skipf("set source owner: %v", err)
	}
	if err := chmodRecoveryUnixMode(filePath, 0o4755); err != nil {
		t.Fatalf("set source setuid mode: %v", err)
	}
	sourceInfo := statRecoveryEntry(t, filePath)
	if sourceInfo.Mode()&os.ModeSetuid == 0 {
		t.Skip("host filesystem does not allow a setuid fixture after ownership change")
	}
	snapshot := filepath.Join(t.TempDir(), "snapshot")
	if err := snapshotCheckout(source, snapshot); err != nil {
		t.Fatalf("snapshotCheckout: %v", err)
	}
	assertRecoveryModeAndIdentity(t, filepath.Join(snapshot, "setuid-file"), 0o4755, 65534, 0, true, false)
}

func TestRecoverySetIDOwnershipFailureRefusesPublication(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires permission to create a destination with a different owner")
	}
	sourcePath := filepath.Join(t.TempDir(), "setuid-source")
	writeRecoveryUnixModeFixture(t, sourcePath, 0o4755)
	sourceInfo := statRecoveryEntry(t, sourcePath)
	if sourceInfo.Mode()&os.ModeSetuid == 0 {
		t.Skip("host filesystem does not allow setuid fixtures")
	}
	targetPath := filepath.Join(t.TempDir(), "private-target")
	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("create private target: %v", err)
	}
	t.Cleanup(func() {
		if err := target.Close(); err != nil {
			t.Errorf("close private target: %v", err)
		}
	})
	if err := target.Chown(65534, -1); err != nil {
		t.Skipf("set destination owner: %v", err)
	}
	called := false
	err = applyRecoveryFileAttributesWith(target, sourceInfo, func(int, int) error {
		called = true
		return errors.New("ownership denied")
	})
	if err == nil || !called {
		t.Fatalf("applyRecoveryFileAttributesWith() = %v, called=%v, want ownership refusal", err, called)
	}
	info, err := target.Stat()
	if err != nil {
		t.Fatalf("stat refused target: %v", err)
	}
	if info.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
		t.Fatalf("refused target retained set-ID bits: %s", info.Mode())
	}
}

func recoveryTestIdentity(t *testing.T, info os.FileInfo) (uid, gid int) {
	t.Helper()
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("file info has unexpected Unix identity type %T", info.Sys())
	}
	return int(stat.Uid), int(stat.Gid)
}

func statRecoveryEntry(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %q: %v", path, err)
	}
	return info
}

func assertRecoveryModeAndIdentity(t *testing.T, path string, mode uint32, uid, gid int, checkUID, checkGID bool) {
	t.Helper()
	info := statRecoveryEntry(t, path)
	want := recoveryUnixFileMode(mode)
	wantMode := want.Perm() | want&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky)
	gotMode := info.Mode().Perm() | info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky)
	if gotMode != wantMode {
		t.Fatalf("mode for %q = %04o, want %04o", path, gotMode, wantMode)
	}
	gotUID, gotGID := recoveryTestIdentity(t, info)
	if checkUID && gotUID != uid {
		t.Fatalf("uid for %q = %d, want %d", path, gotUID, uid)
	}
	if checkGID && gotGID != gid {
		t.Fatalf("gid for %q = %d, want %d", path, gotGID, gid)
	}
}

func writeRecoveryUnixModeFixture(t *testing.T, path string, mode uint32) {
	t.Helper()
	writeRecoveryModeFixture(t, path, 0o600)
	if err := chmodRecoveryUnixMode(path, mode); err != nil {
		t.Fatalf("set Unix fixture mode %q: %v", path, err)
	}
}

func chmodRecoveryUnixMode(path string, mode uint32) error {
	return os.Chmod(path, recoveryUnixFileMode(mode))
}

func recoveryUnixFileMode(mode uint32) os.FileMode {
	fileMode := os.FileMode(mode & 0o777)
	if mode&0o4000 != 0 {
		fileMode |= os.ModeSetuid
	}
	if mode&0o2000 != 0 {
		fileMode |= os.ModeSetgid
	}
	if mode&0o1000 != 0 {
		fileMode |= os.ModeSticky
	}
	return fileMode
}

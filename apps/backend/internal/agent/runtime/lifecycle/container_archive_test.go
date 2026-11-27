package lifecycle

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/stretchr/testify/require"
)

type tarEntry struct {
	name     string
	mode     int64
	typeFlag byte
	uid      int
	gid      int
	body     string
}

func readTarEntries(t *testing.T, archive []byte) []tarEntry {
	t.Helper()
	tr := tar.NewReader(bytes.NewReader(archive))
	var entries []tarEntry
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		body, err := io.ReadAll(tr)
		require.NoError(t, err)
		entries = append(entries, tarEntry{
			name:     hdr.Name,
			mode:     hdr.Mode,
			typeFlag: hdr.Typeflag,
			uid:      hdr.Uid,
			gid:      hdr.Gid,
			body:     string(body),
		})
	}
	return entries
}

func entryByName(entries []tarEntry, name string) (tarEntry, bool) {
	for _, e := range entries {
		if e.name == name {
			return e, true
		}
	}
	return tarEntry{}, false
}

// TestTarUploaderLeavesParentDirectoriesToTheImage keeps delivery from
// rewriting directories the image already owns. The daemon applies a directory
// entry's mode to an existing directory, so an entry for usr/local/bin would
// replace the image's mode; a missing parent is created by the daemon instead.
func TestTarUploaderLeavesParentDirectoriesToTheImage(t *testing.T) {
	up := newTarFileUploader()
	require.NoError(t, up.WriteFile(context.Background(),
		"/home/agent/.config/deep/creds.json", []byte("token"), credentialFileMode))
	require.NoError(t, up.WriteFile(context.Background(),
		remoteAgentctlExecutablePath, []byte("ELF"), 0o755))

	entries := readTarEntries(t, up.Archive())

	for _, entry := range entries {
		require.NotEqual(t, byte(tar.TypeDir), entry.typeFlag,
			"a file must not emit its parent directory %q", entry.name)
	}
	file, ok := entryByName(entries, "home/agent/.config/deep/creds.json")
	require.True(t, ok, "missing file entry in %v", entries)
	require.Equal(t, "token", file.body)
	require.Equal(t, int64(credentialFileMode), file.mode)
}

// TestTarUploaderEnsureDirEmitsOnlyThatDirectory restricts the session-dir
// mode to the directory that asked for it, never to the image's parents.
func TestTarUploaderEnsureDirEmitsOnlyThatDirectory(t *testing.T) {
	up := newTarFileUploader()
	require.NoError(t, up.EnsureDir("/root/.codex"))

	entries := readTarEntries(t, up.Archive())

	require.Len(t, entries, 1, "entries = %v", entries)
	require.Equal(t, "root/.codex/", entries[0].name)
	require.Equal(t, byte(tar.TypeDir), entries[0].typeFlag)
	require.Equal(t, int64(0o700), entries[0].mode)
}

// TestTarUploaderWritesRootOwnedEntries keeps the seeded files readable by the
// container's agent. The backend's own uid means nothing inside the container.
func TestTarUploaderWritesRootOwnedEntries(t *testing.T) {
	up := newTarFileUploader()
	require.NoError(t, up.WriteFile(context.Background(), "/root/.claude/auth.json", []byte("x"), credentialFileMode))

	for _, entry := range readTarEntries(t, up.Archive()) {
		require.Zero(t, entry.uid, "entry %q must be owned by uid 0", entry.name)
		require.Zero(t, entry.gid, "entry %q must be owned by gid 0", entry.name)
	}
}

// TestTarUploaderEmitsEachDirectoryOnce guards against a directory requested
// twice appearing twice, which some extractors reject.
func TestTarUploaderEmitsEachDirectoryOnce(t *testing.T) {
	up := newTarFileUploader()
	ctx := context.Background()
	require.NoError(t, up.EnsureDir("/root/.claude"))
	require.NoError(t, up.EnsureDir("/root/.claude/"))
	require.NoError(t, up.WriteFile(ctx, "/root/.claude/a.json", []byte("a"), credentialFileMode))
	require.NoError(t, up.WriteFile(ctx, "/root/.claude/b.json", []byte("b"), credentialFileMode))

	seen := map[string]int{}
	for _, entry := range readTarEntries(t, up.Archive()) {
		seen[entry.name]++
	}
	require.Equal(t, 0, seen["root/"])
	require.Equal(t, 1, seen["root/.claude/"])
	require.Equal(t, 1, seen["root/.claude/a.json"])
	require.Equal(t, 1, seen["root/.claude/b.json"])
}

// TestTarUploaderAddsExecutable covers the helper binaries, which need the
// executable bit the credential path must never set.
func TestTarUploaderAddsExecutable(t *testing.T) {
	up := newTarFileUploader()
	require.NoError(t, up.WriteFile(context.Background(), remoteAgentctlExecutablePath, []byte("ELF"), 0o755))

	entry, ok := entryByName(readTarEntries(t, up.Archive()), "usr/local/bin/agentctl")
	require.True(t, ok)
	require.Equal(t, int64(0o755), entry.mode)
	require.Equal(t, "ELF", entry.body)
}

// TestTarUploaderIsEmptyUntilWritten keeps a launch that seeds nothing from
// issuing a pointless extraction request.
func TestTarUploaderIsEmptyUntilWritten(t *testing.T) {
	require.True(t, newTarFileUploader().IsEmpty())

	up := newTarFileUploader()
	require.NoError(t, up.WriteFile(context.Background(), "/a.txt", []byte("a"), 0o644))
	require.False(t, up.IsEmpty())
}

// fakeContainerStarter records the daemon calls the create-seed-start sequence
// makes, so the ordering can be asserted without a daemon.
type fakeContainerStarter struct {
	calls      []string
	createID   string
	createErr  error
	startErr   error
	removeErr  error
	removeForc bool
}

func (f *fakeContainerStarter) CreateContainer(_ context.Context, _ docker.ContainerConfig) (string, error) {
	f.calls = append(f.calls, "create")
	return f.createID, f.createErr
}

func (f *fakeContainerStarter) StartContainer(_ context.Context, id string) error {
	f.calls = append(f.calls, "start:"+id)
	return f.startErr
}

func (f *fakeContainerStarter) RemoveContainer(_ context.Context, id string, force bool) error {
	f.calls = append(f.calls, "remove:"+id)
	f.removeForc = force
	return f.removeErr
}

// TestCreateSeedAndStartSeedsBeforeStart is the whole point of the seam: the
// container's inputs have to be in place before its entrypoint runs.
func TestCreateSeedAndStartSeedsBeforeStart(t *testing.T) {
	api := &fakeContainerStarter{createID: "cid-1"}
	var seeded []string

	id, err := createSeedAndStart(context.Background(), api, docker.ContainerConfig{},
		containerCreateHooks{seed: func(_ context.Context, containerID string) error {
			seeded = append(seeded, containerID)
			api.calls = append(api.calls, "seed:"+containerID)
			return nil
		}}, newTestLogger())

	require.NoError(t, err)
	require.Equal(t, "cid-1", id)
	require.Equal(t, []string{"create", "seed:cid-1", "start:cid-1"}, api.calls)
	require.Equal(t, []string{"cid-1"}, seeded)
}

// TestCreateSeedAndStartRemovesContainerWhenSeedingFails keeps a half-built
// container off the daemon. Starting it would run an agent with no helper.
func TestCreateSeedAndStartRemovesContainerWhenSeedingFails(t *testing.T) {
	api := &fakeContainerStarter{createID: "cid-2"}
	seedErr := errors.New("deliver agentctl: connection reset")

	id, err := createSeedAndStart(context.Background(), api, docker.ContainerConfig{},
		containerCreateHooks{seed: func(_ context.Context, _ string) error { return seedErr }}, newTestLogger())

	require.Empty(t, id)
	require.ErrorIs(t, err, seedErr)
	require.Equal(t, []string{"create", "remove:cid-2"}, api.calls)
	require.True(t, api.removeForc, "a created container is removed with force")
}

// TestCreateSeedAndStartWithoutSeederIsUnchanged pins the local Docker path:
// no hook, no extra daemon call.
func TestCreateSeedAndStartWithoutSeederIsUnchanged(t *testing.T) {
	api := &fakeContainerStarter{createID: "cid-3"}

	id, err := createSeedAndStart(context.Background(), api, docker.ContainerConfig{}, containerCreateHooks{}, newTestLogger())

	require.NoError(t, err)
	require.Equal(t, "cid-3", id)
	require.Equal(t, []string{"create", "start:cid-3"}, api.calls)
}

// TestCreateSeedAndStartRemovesContainerWhenStartFails preserves the existing
// failure handling around a start that the seam now sits in front of.
func TestCreateSeedAndStartRemovesContainerWhenStartFails(t *testing.T) {
	api := &fakeContainerStarter{createID: "cid-4", startErr: errors.New("no such image")}

	id, err := createSeedAndStart(context.Background(), api, docker.ContainerConfig{}, containerCreateHooks{}, newTestLogger())

	require.Empty(t, id)
	require.Error(t, err)
	require.Equal(t, []string{"create", "start:cid-4", "remove:cid-4"}, api.calls)
}

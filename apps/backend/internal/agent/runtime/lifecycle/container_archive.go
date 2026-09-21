package lifecycle

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/common/logger"
)

// sessionDirMode is the mode of every directory the archive creates. It
// matches the local path's per-instance session root.
const sessionDirMode int64 = 0o700

// tarFileUploader collects files into one tar archive for delivery into a
// container through the Docker Engine API.
//
// It satisfies FileUploader, so the credential and portable-config seeders run
// against a container exactly as they run against an SSH host, with no
// knowledge of where their bytes end up.
//
// Every entry is written with uid/gid 0. The archive is extracted at "/", where
// the backend's own uid means nothing; the container's agent runs as root, and
// an entry carrying the backend user's uid would be owned by an account the
// container does not have.
type tarFileUploader struct {
	buf  bytes.Buffer
	tw   *tar.Writer
	dirs map[string]bool
	n    int
	err  error
}

func newTarFileUploader() *tarFileUploader {
	up := &tarFileUploader{dirs: map[string]bool{}}
	up.tw = tar.NewWriter(&up.buf)
	return up
}

// WriteFile adds one file and any directories above it that the archive has
// not already created.
func (u *tarFileUploader) WriteFile(_ context.Context, filePath string, data []byte, mode os.FileMode) error {
	if u.err != nil {
		return u.err
	}
	name := archiveName(filePath)
	if name == "" {
		return fmt.Errorf("container archive: %q is not a usable path", filePath)
	}
	if err := u.ensureParents(name); err != nil {
		return err
	}
	if err := u.tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg,
		Name:     name,
		Mode:     int64(mode.Perm()),
		Size:     int64(len(data)),
		ModTime:  time.Unix(0, 0),
	}); err != nil {
		u.err = fmt.Errorf("container archive: write header for %s: %w", name, err)
		return u.err
	}
	if _, err := u.tw.Write(data); err != nil {
		u.err = fmt.Errorf("container archive: write %s: %w", name, err)
		return u.err
	}
	u.n++
	return nil
}

// EnsureDir adds a directory with no file in it, for an agent whose session
// directory must exist before its own setup writes into it.
func (u *tarFileUploader) EnsureDir(dirPath string) error {
	if u.err != nil {
		return u.err
	}
	name := archiveName(dirPath)
	if name == "" {
		return fmt.Errorf("container archive: %q is not a usable directory", dirPath)
	}
	if err := u.ensureParents(name); err != nil {
		return err
	}
	return u.writeDir(name + "/")
}

func (u *tarFileUploader) ensureParents(name string) error {
	dir := path.Dir(name)
	if dir == "." || dir == "/" {
		return nil
	}
	var components []string
	for _, part := range strings.Split(dir, "/") {
		if part == "" {
			continue
		}
		components = append(components, part)
		if err := u.writeDir(strings.Join(components, "/") + "/"); err != nil {
			return err
		}
	}
	return nil
}

func (u *tarFileUploader) writeDir(name string) error {
	if u.dirs[name] {
		return nil
	}
	if err := u.tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     name,
		Mode:     sessionDirMode,
		ModTime:  time.Unix(0, 0),
	}); err != nil {
		u.err = fmt.Errorf("container archive: write directory %s: %w", name, err)
		return u.err
	}
	u.dirs[name] = true
	u.n++
	return nil
}

// IsEmpty reports whether anything has been added, so a launch that seeds
// nothing skips the extraction request.
func (u *tarFileUploader) IsEmpty() bool { return u.n == 0 }

// Archive closes the writer and returns the tar stream. It is single-use: the
// bytes include the archive trailer, so nothing may be added afterward.
func (u *tarFileUploader) Archive() []byte {
	if u.tw != nil {
		if err := u.tw.Close(); err != nil && u.err == nil {
			u.err = fmt.Errorf("container archive: close: %w", err)
		}
		u.tw = nil
	}
	return u.buf.Bytes()
}

// Err reports the first failure, if any, so a caller can check once at the end
// instead of at every write.
func (u *tarFileUploader) Err() error { return u.err }

// archiveName turns an absolute container path into a tar entry name. Tar
// entries are relative, and the archive is extracted at "/".
func archiveName(p string) string {
	cleaned := path.Clean("/" + strings.ReplaceAll(p, "\\", "/"))
	return strings.TrimPrefix(cleaned, "/")
}

// containerStarter is the subset of the daemon API the create-seed-start
// sequence uses, narrow enough to drive the sequence without a daemon.
// containerCreateHooks are the steps that run between creating a container and
// starting it, in that order. A nil hook is skipped and makes no daemon call.
type containerCreateHooks struct {
	// attachNetworks connects the container's additional networks.
	attachNetworks func(ctx context.Context, containerID string) error
	// seed delivers the container's inputs.
	seed func(ctx context.Context, containerID string) error
}

type containerStarter interface {
	CreateContainer(ctx context.Context, cfg docker.ContainerConfig) (string, error)
	StartContainer(ctx context.Context, containerID string) error
	RemoveContainer(ctx context.Context, containerID string, force bool) error
}

// createSeedAndStart creates a container, delivers its inputs, and starts it.
//
// Seeding sits between create and start because a created container already
// has a filesystem the daemon extracts into, and because the container's
// entrypoint must not run before its helper and credentials are in place.
// A container that cannot be seeded is removed: starting it would run an agent
// with no helper, which fails later and names nothing useful.
func createSeedAndStart(
	ctx context.Context,
	api containerStarter,
	cfg docker.ContainerConfig,
	hooks containerCreateHooks,
	log *logger.Logger,
) (string, error) {
	containerID, err := api.CreateContainer(ctx, cfg)
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	// Attaching precedes seeding because the seed extracts the agent's
	// credentials into the container. A network failure after that point would
	// have already placed them in a container about to be removed.
	for _, hook := range []func(context.Context, string) error{hooks.attachNetworks, hooks.seed} {
		if hook == nil {
			continue
		}
		if hookErr := hook(ctx, containerID); hookErr != nil {
			removeContainerAfterFailure(api, containerID, log)
			return "", hookErr
		}
	}

	if err := api.StartContainer(ctx, containerID); err != nil {
		removeContainerAfterFailure(api, containerID, log)
		return "", fmt.Errorf("failed to start container: %w", err)
	}
	return containerID, nil
}

func removeContainerAfterFailure(api containerStarter, containerID string, log *logger.Logger) {
	if containerID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := api.RemoveContainer(ctx, containerID, true); err != nil && log != nil {
		log.Warn("failed to remove container after launch failure",
			zap.String("container_id", containerID),
			zap.Error(err))
	}
}

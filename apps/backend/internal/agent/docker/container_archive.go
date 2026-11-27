package docker

import (
	"bytes"
	"context"
	"fmt"

	"github.com/moby/moby/client"
	"go.uber.org/zap"
)

// CopyToContainer extracts a tar archive into a container's filesystem at
// dstPath.
//
// The container does not have to be running: a created container already has
// the filesystem the daemon extracts into, which is what lets a remote daemon
// receive container inputs that no bind mount can supply. dstPath must already
// exist in the container, so a caller delivering into directories the image
// does not have extracts at "/" and carries those directories in the archive.
func (c *Client) CopyToContainer(ctx context.Context, containerID, dstPath string, archive []byte) error {
	if containerID == "" {
		return fmt.Errorf("copy to container: container ID is required")
	}
	if dstPath == "" {
		return fmt.Errorf("copy to container %s: destination path is required", containerID)
	}

	c.logger.Debug("Copying archive into container",
		zap.String("container_id", containerID),
		zap.String("destination", dstPath),
		zap.Int("bytes", len(archive)))

	if _, err := c.cli.CopyToContainer(ctx, containerID, client.CopyToContainerOptions{
		DestinationPath: dstPath,
		Content:         bytes.NewReader(archive),
	}); err != nil {
		return c.ExplainRemoteFailure(
			fmt.Errorf("copy archive into container %s at %s: %w", containerID, dstPath, err))
	}
	return nil
}

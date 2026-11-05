// Package compatibility checks installed SDK evidence against a plugin manifest.
// It does not replace the host's complete manifest validator.
package compatibility

import (
	"bytes"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"
	"gopkg.in/yaml.v3"
)

func CheckManifest(data []byte, records ...Record) error {
	if len(records) == 0 {
		return fmt.Errorf("no consumed SDK was selected")
	}
	if len(data) > 1<<20 {
		return fmt.Errorf("manifest exceeds 1 MiB")
	}
	var manifest struct {
		Minimum string `yaml:"min_kandev_version"`
		API     int    `yaml:"api_version"`
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&manifest); err != nil {
		return fmt.Errorf("manifest: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return fmt.Errorf("manifest must contain one YAML document")
	}
	minimum, err := stableVersion("manifest min_kandev_version", strings.TrimPrefix(manifest.Minimum, "v"))
	if err != nil {
		return err
	}
	for _, record := range records {
		if err := record.validate(record.SDKVersion); err != nil {
			return err
		}
		floor, _ := semver.StrictNewVersion(record.Minimum)
		if minimum.LessThan(floor) {
			return fmt.Errorf("SDK %s requires min_kandev_version >= %s; manifest declares %s", record.SDKVersion, record.Minimum, manifest.Minimum)
		}
		if !slices.Contains(record.APIVersions, manifest.API) {
			return fmt.Errorf("SDK %s does not support manifest api_version %d", record.SDKVersion, manifest.API)
		}
	}
	return nil
}

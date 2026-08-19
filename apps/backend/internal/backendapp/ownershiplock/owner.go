package ownershiplock

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/kandev/kandev/internal/common/proclive"
)

// OwnerRecord is advisory diagnostic metadata about the process that acquired
// a lock. It is never consulted to decide ownership, staleness, or takeover.
type OwnerRecord struct {
	PID        int64  `json:"pid"`
	Executable string `json:"executable"`
	StartedAt  string `json:"started_at"`
}

var (
	writeOwnerFn    = writeOwner
	ownerLivenessFn = proclive.Alive
)

const (
	// Windows locks the first byte of the file. Keep metadata after that byte
	// so a conflicting process can read it through a separate file handle.
	ownerMetadataOffset    int64 = 1
	maxOwnerMetadataLength int64 = 4096
)

func writeOwner(file *os.File) error {
	executable, _ := os.Executable()
	record := OwnerRecord{
		PID:        int64(os.Getpid()),
		Executable: executable,
		StartedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if err := file.Truncate(0); err != nil {
		return err
	}
	n, err := file.WriteAt(data, ownerMetadataOffset)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	return nil
}

func readOwner(path string) *OwnerRecord {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil
	}
	metadataLength := info.Size() - ownerMetadataOffset
	if metadataLength <= 0 || metadataLength > maxOwnerMetadataLength {
		return nil
	}
	data := make([]byte, int(metadataLength))
	if _, err := file.ReadAt(data, ownerMetadataOffset); err != nil {
		return nil
	}
	var record OwnerRecord
	if err := json.Unmarshal(data, &record); err != nil || record.PID <= 0 {
		return nil
	}
	alive, known := ownerLivenessFn(record.PID)
	if known && !alive {
		return nil
	}
	return &record
}

func (r *OwnerRecord) String() string {
	if r == nil {
		return ""
	}
	detail := fmt.Sprintf("pid %d", r.PID)
	if r.Executable != "" {
		detail += ", " + r.Executable
	}
	if r.StartedAt != "" {
		detail += ", started " + r.StartedAt
	}
	return detail
}

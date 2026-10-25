package websocket

import (
	"encoding/base64"
	"fmt"
	"strconv"
)

const lspLeaseSnapshotFixedOverhead = 1024

func (l *lspLease) checkSnapshotLimitLocked() error {
	size := lspLeaseSnapshotFixedOverhead + len(l.initializeResult) + len(l.initializeServerID) + len(l.initializeResponse)
	size += l.configurationSnapshotBytes + l.documentVersionsSnapshotBytes
	size += l.registrationSnapshotBytes + l.progressTokenSnapshotBytes + l.progressSnapshotBytes
	size += boundedJSONStringSize(l.workspacePath) + boundedJSONStringSize(l.workspaceURI)
	for _, path := range l.repoSubpaths {
		size += boundedJSONStringSize(path) + 1
	}
	if l.initializeWaiter != nil {
		size += base64.StdEncoding.EncodedLen(len(l.initializeWaiter.clientID)) + 128
	}
	size += len(strconv.Itoa(len(l.clientRequests))) + len(strconv.Itoa(l.pendingClientRequestBytes))
	if size > lspLeaseSnapshotLimit {
		return fmt.Errorf("LSP resume state is at least %d bytes; maximum is %d", size, lspLeaseSnapshotLimit)
	}
	return nil
}

func boundedJSONStringSize(value string) int {
	return len(value)*6 + 2
}

func snapshotByteSliceMapEntrySize(key string, value []byte) int {
	return boundedJSONStringSize(key) + base64.StdEncoding.EncodedLen(len(value)) + 4
}

func snapshotDocumentVersionEntrySize(uri string, version int64) int {
	return boundedJSONStringSize(uri) + len(strconv.FormatInt(version, 10)) + 2
}

func storeSnapshotByteSlice(values map[string][]byte, key string, value []byte, size *int) {
	if previous, exists := values[key]; exists {
		*size -= snapshotByteSliceMapEntrySize(key, previous)
	}
	stored := append([]byte(nil), value...)
	values[key] = stored
	*size += snapshotByteSliceMapEntrySize(key, stored)
}

func deleteSnapshotByteSlice(values map[string][]byte, key string, size *int) {
	if previous, exists := values[key]; exists {
		*size -= snapshotByteSliceMapEntrySize(key, previous)
		delete(values, key)
	}
}

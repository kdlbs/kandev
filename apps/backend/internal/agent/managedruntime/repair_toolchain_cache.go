package managedruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

const repairNPMVersionCacheLimit = 64

type repairNPMVersionCacheEntry struct {
	key     string
	version string
}

var repairNPMVersionFacts = struct {
	sync.Mutex
	entries []repairNPMVersionCacheEntry
}{}

func repairNPMVersionFactKey(executable string, env []string) (string, error) {
	executableInfo, err := os.Stat(executable)
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	writeFingerprint := func(value string) {
		_, _ = digest.Write([]byte(value))
		_, _ = digest.Write([]byte{0})
	}
	writeFingerprint(executable)
	writeFingerprint(executableInfo.ModTime().String())
	writeFingerprint(strconv.FormatInt(executableInfo.Size(), 10))
	writeFingerprint(repairNodeExecutableIdentity(env))
	for _, key := range []string{"NODE_OPTIONS", "NODE_PATH"} {
		writeFingerprint(key)
		writeFingerprint(repairEnvValue(env, key))
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func repairNodeExecutableIdentity(env []string) string {
	searchPath := repairEnvValue(env, "PATH")
	if searchPath == "" {
		return ""
	}
	node, err := repairFindExecutable(searchPath, "node")
	if err != nil {
		return ""
	}
	if resolved, resolveErr := filepath.EvalSymlinks(node); resolveErr == nil {
		node = resolved
	}
	info, err := os.Stat(node)
	if err != nil {
		return ""
	}
	return info.Name() + "|" + node + "|" + info.ModTime().String()
}

func readRepairNPMVersionFact(key string) (string, bool) {
	repairNPMVersionFacts.Lock()
	defer repairNPMVersionFacts.Unlock()
	for _, entry := range repairNPMVersionFacts.entries {
		if entry.key == key {
			return entry.version, true
		}
	}
	return "", false
}

func writeRepairNPMVersionFact(key, version string) {
	repairNPMVersionFacts.Lock()
	defer repairNPMVersionFacts.Unlock()
	for index, entry := range repairNPMVersionFacts.entries {
		if entry.key == key {
			repairNPMVersionFacts.entries[index].version = version
			return
		}
	}
	if len(repairNPMVersionFacts.entries) >= repairNPMVersionCacheLimit {
		copy(repairNPMVersionFacts.entries, repairNPMVersionFacts.entries[1:])
		repairNPMVersionFacts.entries[len(repairNPMVersionFacts.entries)-1] = repairNPMVersionCacheEntry{key: key, version: version}
		return
	}
	repairNPMVersionFacts.entries = append(repairNPMVersionFacts.entries, repairNPMVersionCacheEntry{key: key, version: version})
}

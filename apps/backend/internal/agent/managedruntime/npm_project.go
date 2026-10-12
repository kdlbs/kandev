package managedruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// NPMProjectPrefix is the trusted command marker replaced with an isolated
// system-temporary project root before a managed npm process starts.
const NPMProjectPrefix = "~/.kandev/managed-npm-runtime"

// NPMProjectPrefixArgs returns the canonical npm project root arguments used
// by every managed runtime command. PrepareNPMProjectPrefix resolves the
// marker on the execution host before the command starts.
func NPMProjectPrefixArgs() []string {
	return []string{"--prefix", NPMProjectPrefix}
}

// PrepareNPMProjectPrefix replaces the trusted marker with a private directory
// under the execution host's system temp root. The home directory can itself
// be a bind-mounted agent session, so managed npm project state must stay out
// of it as well as out of the task workspace.
func PrepareNPMProjectPrefix(args []string) error {
	index := npmProjectPrefixArgumentIndex(args)
	if index < 0 {
		return nil
	}

	prefix, err := npmProjectPrefixPath()
	if err != nil {
		return errors.New("managed npm project prefix could not be prepared")
	}
	if err := os.MkdirAll(prefix, 0o700); err != nil {
		return errors.New("managed npm project prefix could not be prepared")
	}
	info, err := os.Lstat(prefix)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("managed npm project prefix could not be prepared")
	}
	if err := os.Chmod(prefix, 0o700); err != nil {
		return errors.New("managed npm project prefix could not be prepared")
	}
	args[index+1] = prefix
	return nil
}

func npmProjectPrefixArgumentIndex(args []string) int {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == "--prefix" && args[index+1] == NPMProjectPrefix {
			return index
		}
	}
	return -1
}

func npmProjectPrefixPath() (string, error) {
	tempRoot := os.TempDir()
	if strings.TrimSpace(tempRoot) == "" || !filepath.IsAbs(tempRoot) {
		return "", errors.New("system temporary root is unavailable")
	}
	current, err := user.Current()
	if err != nil || current == nil || strings.TrimSpace(current.Uid) == "" {
		return "", errors.New("execution user is unavailable")
	}
	userHash := sha256.Sum256([]byte(current.Uid))
	scope := hex.EncodeToString(userHash[:8])
	return filepath.Join(tempRoot, "kandev-managed-npm-runtime-"+scope), nil
}

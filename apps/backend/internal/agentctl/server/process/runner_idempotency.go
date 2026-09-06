package process

import (
	"fmt"
	"sort"
	"strings"
)

// ErrProcessRequestIdentityConflict means an idempotency key was reused with
// inputs different from the process it originally admitted.
var ErrProcessRequestIdentityConflict = fmt.Errorf("process request identity conflict")

const maxRetainedIdempotentProcesses = 256

func processRequestFingerprint(req StartProcessRequest) string {
	keys := make([]string, 0, len(req.Env))
	for key := range req.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var fingerprint strings.Builder
	fingerprint.WriteString(req.SessionID)
	fingerprint.WriteByte(0)
	fingerprint.WriteString(string(req.Kind))
	fingerprint.WriteByte(0)
	fingerprint.WriteString(req.ScriptName)
	fingerprint.WriteByte(0)
	fingerprint.WriteString(req.Command)
	fingerprint.WriteByte(0)
	fingerprint.WriteString(req.WorkingDir)
	fingerprint.WriteByte(0)
	fingerprint.WriteString(req.Timeout.String())
	fingerprint.WriteByte(0)
	fingerprint.WriteString(fmt.Sprint(req.BufferMaxBytes))
	for _, key := range keys {
		fingerprint.WriteByte(0)
		fingerprint.WriteString(key)
		fingerprint.WriteByte('=')
		fingerprint.WriteString(req.Env[key])
	}
	return fingerprint.String()
}

package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const profileProbeEvidenceEnv = "MOCK_AGENT_PROFILE_EVIDENCE_FILE"

type profileProbeEvidence struct {
	Wrapper     bool     `json:"wrapper"`
	Command     string   `json:"command,omitempty"`
	EnvCatalog  string   `json:"env_catalog,omitempty"`
	CLICatalog  string   `json:"cli_catalog,omitempty"`
	ProfileArgs []string `json:"profile_args,omitempty"`
}

func runProfileProbeWrapper(args []string, evidencePath string) int {
	if len(args) < 1 || args[0] == "" || evidencePath == "" {
		return 2
	}
	if err := appendProfileProbeEvidence(evidencePath, profileProbeEvidence{
		Wrapper: true,
		Command: filepath.Base(args[0]),
	}); err != nil {
		return 2
	}
	command := exec.Command(args[0], args[1:]...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			return exitError.ExitCode()
		}
		return 1
	}
	return 0
}

func recordProfileProbeChildEvidence(args []string, evidencePath string) {
	if evidencePath == "" {
		return
	}
	_ = appendProfileProbeEvidence(evidencePath, profileProbeEvidence{
		Command:     filepath.Base(args[0]),
		EnvCatalog:  os.Getenv("MOCK_AGENT_PROFILE_CATALOG"),
		CLICatalog:  profileCatalogFlag(args),
		ProfileArgs: profileCatalogArgs(args),
	})
}

func profileCatalogArgs(args []string) []string {
	var result []string
	for index := 1; index < len(args); index++ {
		if strings.HasPrefix(args[index], "--profile-catalog=") {
			result = append(result, args[index])
			continue
		}
		if args[index] == "--profile-catalog" && index+1 < len(args) {
			result = append(result, args[index], args[index+1])
			index++
		}
	}
	return result
}

func appendProfileProbeEvidence(path string, evidence profileProbeEvidence) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	encodeErr := json.NewEncoder(file).Encode(evidence)
	closeErr := file.Close()
	if encodeErr != nil {
		return encodeErr
	}
	return closeErr
}

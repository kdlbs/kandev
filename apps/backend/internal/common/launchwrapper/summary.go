package launchwrapper

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

type CommandSummary struct {
	Executable       string   `json:"executable"`
	OmittedArguments int      `json:"omitted_arguments"`
	References       []string `json:"references"`
}

type Summary struct {
	Source  string         `json:"source"`
	Digest  string         `json:"digest"`
	Command CommandSummary `json:"command"`
}

// Summary never includes opaque argv values or executable directory paths.
func (s Spec) Summary() (*Summary, error) {
	if s.IsZero() {
		return nil, nil
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(raw)
	return &Summary{Source: s.Source, Digest: hex.EncodeToString(digest[:]), Command: SummarizeCommand(s.Argv)}, nil
}

func SummarizeCommand(argv []string) CommandSummary {
	result := CommandSummary{References: []string{}}
	if len(argv) == 0 {
		return result
	}
	result.Executable = safeExecutableName(argv[0])
	for _, value := range argv[1:] {
		if value == WorkDir || value == ManagedNPMDir {
			result.References = append(result.References, value)
		} else {
			result.OmittedArguments++
		}
	}
	return result
}

func safeExecutableName(value string) string {
	name := value[strings.LastIndexAny(value, `/\`)+1:]
	if len(name) == 0 || len(name) > 128 || name == "." || name == ".." {
		return ""
	}
	for _, char := range name {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("._-", char) {
			continue
		}
		return ""
	}
	return name
}

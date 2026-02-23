package remoteauth

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/kandev/kandev/internal/agent/agents"
)

const (
	ghCLISpecID          = "gh_cli"
	ghCLITokenMethodID   = "gh_cli_token"
	ghCLIEnvMethodID     = "gh_cli_env"
	ghCLIEnvVar          = "GITHUB_TOKEN"
	ghCLIDisplayName     = "GitHub CLI"
	ghCLITokenMethodType = "gh_cli_token"
)

// Method describes one selectable remote authentication method.
type Method struct {
	MethodID      string   `json:"method_id"`
	Type          string   `json:"type"`
	EnvVar        string   `json:"env_var,omitempty"`
	SetupHint     string   `json:"setup_hint,omitempty"`
	SourceFiles   []string `json:"source_files,omitempty"`
	TargetRelDir  string   `json:"target_rel_dir,omitempty"`
	Label         string   `json:"label,omitempty"`
	HasLocalFiles bool     `json:"has_local_files,omitempty"`
}

// Spec groups auth methods for one integration or agent.
type Spec struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"display_name"`
	Methods     []Method `json:"methods"`
}

// Catalog contains remote auth specs and fast lookup by method ID.
type Catalog struct {
	Specs       []Spec
	methodsByID map[string]Method
}

// BuildCatalog creates a host-aware remote auth catalog for all enabled agents.
func BuildCatalog(enabledAgents []agents.Agent) Catalog {
	homeDir, _ := os.UserHomeDir()
	return BuildCatalogForHost(enabledAgents, runtime.GOOS, homeDir)
}

// BuildCatalogForHost creates a remote auth catalog for an explicit OS/home.
func BuildCatalogForHost(enabledAgents []agents.Agent, currentOS, homeDir string) Catalog {
	specs := make([]Spec, 0, len(enabledAgents)+1)
	methodsByID := map[string]Method{}

	ghSpec := Spec{
		ID:          ghCLISpecID,
		DisplayName: ghCLIDisplayName,
		Methods: []Method{
			{
				MethodID:  ghCLITokenMethodID,
				Type:      ghCLITokenMethodType,
				Label:     "Copy token from local gh CLI",
				SetupHint: "Runs `gh auth token` on your machine and sets GITHUB_TOKEN in the remote environment",
			},
			{
				MethodID: ghCLIEnvMethodID,
				Type:     "env",
				EnvVar:   ghCLIEnvVar,
				Label:    "Provide a GitHub token",
			},
		},
	}
	specs = append(specs, ghSpec)
	for _, method := range ghSpec.Methods {
		methodsByID[method.MethodID] = method
	}

	for _, ag := range enabledAgents {
		auth := ag.RemoteAuth()
		if auth == nil {
			continue
		}

		spec := Spec{
			ID:          ag.ID(),
			DisplayName: ag.DisplayName(),
		}

		fileMethodIndex := 0
		for _, method := range auth.Methods {
			switch method.Type {
			case "files":
				files := method.SourceFiles[currentOS]
				entry := Method{
					MethodID:      fmt.Sprintf("agent:%s:files:%d", ag.ID(), fileMethodIndex),
					Type:          "files",
					SourceFiles:   files,
					TargetRelDir:  strings.Trim(method.TargetRelDir, "/"),
					Label:         method.Label,
					HasLocalFiles: anyFileExists(homeDir, files),
				}
				spec.Methods = append(spec.Methods, entry)
				methodsByID[entry.MethodID] = entry
				fileMethodIndex++
			case "env":
				if method.EnvVar == "" {
					continue
				}
				entry := Method{
					MethodID:  fmt.Sprintf("agent:%s:env:%s", ag.ID(), method.EnvVar),
					Type:      "env",
					EnvVar:    method.EnvVar,
					SetupHint: method.SetupHint,
					Label:     method.Label,
				}
				spec.Methods = append(spec.Methods, entry)
				methodsByID[entry.MethodID] = entry
			}
		}

		if len(spec.Methods) > 0 {
			specs = append(specs, spec)
		}
	}

	return Catalog{Specs: specs, methodsByID: methodsByID}
}

// FindMethod returns a method by ID.
func (c Catalog) FindMethod(methodID string) (Method, bool) {
	method, ok := c.methodsByID[methodID]
	return method, ok
}

func anyFileExists(homeDir string, files []string) bool {
	if homeDir == "" {
		return false
	}
	for _, file := range files {
		if _, err := os.Stat(filepath.Join(homeDir, file)); err == nil {
			return true
		}
	}
	return false
}

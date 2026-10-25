package sandbox

const ReferenceWorkDir = "work_dir"
const ReferenceAgentState = "agent_state"

// ResolveWorkDirectory binds logical work grants without allocating agent state.
func ResolveWorkDirectory(grants Grants, workDir string, protectedRoots []string) (Grants, error) {
	return ResolveRuntimeDirectories(grants, workDir, nil, protectedRoots)
}

// ResolveRuntimeDirectories grants only an owned child directory inside protected
// storage. Literal paths and work directories cannot use this exception.
func ResolveRuntimeDirectories(grants Grants, workDir string, state *AgentState, protectedRoots []string) (Grants, error) {
	resolved, err := Canonicalize(grants, protectedRoots)
	if err != nil {
		return Grants{}, err
	}
	for index, grant := range resolved.Filesystem {
		switch grant.Reference {
		case ReferenceAgentState:
			if !state.valid() {
				return Grants{}, ErrInvalidPolicy
			}
			resolved.Filesystem[index] = FilesystemGrant{Path: state.Directories().Home, Writable: grant.Writable}
		case ReferenceWorkDir:
			value, err := Canonicalize(Grants{Filesystem: []FilesystemGrant{{Path: workDir, Writable: grant.Writable}}}, protectedRoots)
			if err != nil {
				return Grants{}, err
			}
			resolved.Filesystem[index] = value.Filesystem[0]
		}
	}
	return Canonicalize(resolved, nil)
}

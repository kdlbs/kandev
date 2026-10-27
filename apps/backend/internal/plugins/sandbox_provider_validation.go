package plugins

import (
	"errors"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/protobuf/proto"
)

const (
	maxSandboxMessageBytes  = pluginsdk.MaxSandboxMessageBytes
	sandboxApprovalEndpoint = "sandbox.approval.endpoint"
	sandboxApprovalToken    = "sandbox.approval.token"
	sandboxEgressEndpoint   = "sandbox.egress.endpoint"
)

var errSandboxProviderReply = errors.New("plugins: invalid sandbox provider reply")

func validateSandboxCompilation(req *pluginsdk.CompileSandboxPolicyRequest, compiled *pluginsdk.SandboxCompiledPolicy) error {
	if req == nil || req.Policy == nil || compiled == nil || compiled.EffectiveGrants == nil ||
		!proto.Equal(req.Policy.Grants, compiled.EffectiveGrants) || proto.Size(compiled) > maxSandboxMessageBytes {
		return errSandboxProviderReply
	}
	artifacts, err := validateSandboxArtifacts(compiled.Artifacts)
	if err != nil {
		return err
	}
	if err := validateSandboxArtifactBindings(req, compiled.Artifacts); err != nil {
		return err
	}
	for _, name := range compiled.StatusArtifacts {
		if !artifacts[name] {
			return errSandboxProviderReply
		}
	}
	return validateSandboxPrefix(req, compiled.Prefix, artifacts)
}

func validateSandboxArtifactBindings(req *pluginsdk.CompileSandboxPolicyRequest, artifacts []*pluginsdk.SandboxArtifact) error {
	values := map[string]string{sandboxApprovalEndpoint: "", sandboxApprovalToken: "", sandboxEgressEndpoint: ""}
	for _, reference := range req.References {
		if reference != nil && !privateSandboxReference(reference.Key) {
			values[reference.Key] = reference.Value
		}
	}
	for _, artifact := range artifacts {
		if len(artifact.Bindings) == 0 {
			continue
		}
		bindings := make([]sandbox.ArtifactBinding, 0, len(artifact.Bindings))
		for _, binding := range artifact.Bindings {
			if binding == nil {
				return errSandboxProviderReply
			}
			bindings = append(bindings, sandbox.ArtifactBinding{Path: binding.Path, Reference: binding.RuntimeReference})
		}
		if _, err := sandbox.RenderJSONArtifact(artifact.ContentUtf8, bindings, values); err != nil {
			return errSandboxProviderReply
		}
	}
	return nil
}

func validateSandboxArtifacts(values []*pluginsdk.SandboxArtifact) (map[string]bool, error) {
	if len(values) > 32 {
		return nil, errSandboxProviderReply
	}
	seen := make(map[string]bool, len(values))
	for _, artifact := range values {
		if artifact == nil || !validSandboxArtifactName(artifact.Name) || seen[artifact.Name] ||
			(artifact.Kind != pluginsdk.SandboxArtifactProfile && artifact.Kind != pluginsdk.SandboxArtifactConfig) || !utf8.ValidString(artifact.ContentUtf8) ||
			strings.ContainsRune(artifact.ContentUtf8, 0) {
			return nil, errSandboxProviderReply
		}
		seen[artifact.Name] = true
	}
	return seen, nil
}

func validSandboxArtifactName(name string) bool {
	return name != "" && len(name) <= 255 && name != "." && name != ".." &&
		!path.IsAbs(name) && path.Clean(name) == name && !strings.HasPrefix(name, "../") &&
		!strings.ContainsAny(name, "\\:") && utf8.ValidString(name) && strings.IndexFunc(name, unicode.IsControl) == -1
}

func validateSandboxPrefix(req *pluginsdk.CompileSandboxPolicyRequest, prefix []*pluginsdk.SandboxArgument, artifacts map[string]bool) error {
	if len(prefix) == 0 || len(prefix) > 128 || prefix[0].GetRuntimeReference() != pluginsdk.SandboxRuntimeReferenceKey {
		return errSandboxProviderReply
	}
	references := map[string]bool{
		pluginsdk.SandboxRuntimeReferenceKey: true,
	}
	for _, ref := range req.References {
		if ref != nil && !privateSandboxReference(ref.Key) {
			references[ref.Key] = true
		}
	}
	for _, arg := range prefix {
		if !validSandboxArgument(arg, references, artifacts) {
			return errSandboxProviderReply
		}
	}
	return nil
}

func privateSandboxReference(key string) bool {
	return key == sandboxApprovalEndpoint || key == sandboxApprovalToken || key == sandboxEgressEndpoint
}

func validSandboxRequestPayload(request proto.Message) bool {
	compile, ok := request.(*pluginsdk.CompileSandboxPolicyRequest)
	if !ok {
		return true
	}
	for _, reference := range compile.References {
		if reference == nil || privateSandboxReference(reference.Key) {
			return false
		}
	}
	return true
}

func validSandboxArgument(arg *pluginsdk.SandboxArgument, references, artifacts map[string]bool) bool {
	if arg == nil {
		return false
	}
	switch value := arg.Value.(type) {
	case *pluginsdk.SandboxArgument_Literal:
		return !strings.ContainsRune(value.Literal, 0) && utf8.ValidString(value.Literal)
	case *pluginsdk.SandboxArgument_RuntimeReference:
		return references[value.RuntimeReference]
	case *pluginsdk.SandboxArgument_ArtifactReference:
		return artifacts[value.ArtifactReference]
	default:
		return false
	}
}

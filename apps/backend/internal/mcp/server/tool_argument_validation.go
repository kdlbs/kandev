package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/mcp/toolschema"
	"github.com/mark3labs/mcp-go/mcp"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"go.uber.org/zap"
)

type toolArgumentValidator struct {
	schema *jsonschema.Schema
	err    error
}

func (s *Server) rebuildToolArgumentValidators() {
	tools := s.mcpServer.ListTools()
	validators := make(map[string]toolArgumentValidator, len(tools))

	for name, serverTool := range tools {
		schema, err := compileToolArgumentSchema(name, serverTool.Tool)
		validators[name] = toolArgumentValidator{schema: schema, err: err}
		if err != nil {
			s.logger.Error("failed to compile MCP tool argument schema",
				zap.String("tool", name),
				zap.Error(err))
		}
	}

	s.validatorMu.Lock()
	s.toolValidators = validators
	s.validatorMu.Unlock()
}

func compileToolArgumentSchema(toolName string, tool mcp.Tool) (*jsonschema.Schema, error) {
	rawSchema := tool.RawInputSchema
	if rawSchema == nil {
		var err error
		rawSchema, err = json.Marshal(tool.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("marshal schema: %w", err)
		}
	}

	var schemaDoc map[string]any
	if err := json.Unmarshal(rawSchema, &schemaDoc); err != nil {
		return nil, fmt.Errorf("decode schema: %w", err)
	}
	if err := toolschema.CheckPortableRoot(schemaDoc); err != nil {
		return nil, err
	}
	schemaDoc["additionalProperties"] = false

	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft7)
	resourceURL := "https://kandev.local/mcp/schemas/" + url.PathEscape(toolName)
	if err := compiler.AddResource(resourceURL, schemaDoc); err != nil {
		return nil, fmt.Errorf("add schema resource: %w", err)
	}
	schema, err := compiler.Compile(resourceURL)
	if err != nil {
		return nil, fmt.Errorf("compile schema: %w", err)
	}
	return schema, nil
}

func (s *Server) validateToolArguments(toolName string, req mcp.CallToolRequest) (mcp.CallToolRequest, error) {
	arguments := req.GetRawArguments()
	if raw, ok := arguments.(json.RawMessage); ok {
		var decoded any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return req, fmt.Errorf("invalid arguments for %s: failed to decode arguments", toolName)
		}
		arguments = decoded
	}
	if arguments == nil {
		arguments = map[string]any{}
	}
	arguments, err := normalizeToolArguments(toolName, arguments)
	if err != nil {
		return req, err
	}
	req.Params.Arguments = arguments

	// SetMode replaces the MCP tool set before rebuilding its validators. Use
	// the server read lock as a barrier so validation cannot observe that
	// intermediate state or pair a newly selected handler with old validators.
	s.mu.RLock()
	defer s.mu.RUnlock()

	s.validatorMu.RLock()
	validator, ok := s.toolValidators[toolName]
	s.validatorMu.RUnlock()
	if !ok {
		return req, fmt.Errorf("invalid arguments for %s: schema validator is unavailable", toolName)
	}
	if validator.err != nil || validator.schema == nil {
		return req, fmt.Errorf("invalid arguments for %s: registered schema is invalid", toolName)
	}
	if err := validator.schema.Validate(arguments); err != nil {
		return req, sanitizedToolArgumentError(toolName, err)
	}
	return req, nil
}

func sanitizedToolArgumentError(toolName string, err error) error {
	var validationErr *jsonschema.ValidationError
	if !errors.As(err, &validationErr) {
		return fmt.Errorf("invalid arguments for %s: validation failed", toolName)
	}

	failure, keyword := firstKeywordFailure(validationErr)
	if failure == nil {
		failure = validationErr
		keyword = "schema"
	}
	primaryPath := validationInstancePath(failure.InstanceLocation)
	return fmt.Errorf("invalid arguments for %s: validation failed at %s (keyword: %s%s%s)",
		toolName, primaryPath, keyword,
		missingRequiredProperties(validationErr, primaryPath),
		unknownArgumentDetail(toolName, validationErr, primaryPath))
}

// sessionBoundTaskTools identifies tools whose task scope is owned by the
// calling session and cannot be selected through arguments.
var sessionBoundTaskTools = map[string]bool{
	"get_task_change_requests_kandev":              true,
	"update_task_change_request_automation_kandev": true,
}

const sessionBoundTaskRule = "This tool is bound to the calling task; cross-task targeting is not supported."

// unknownArgumentDetail traverses every validation failure and groups rejected
// properties by instance path before it formats the diagnostic.
func unknownArgumentDetail(toolName string, root *jsonschema.ValidationError, primaryPath string) string {
	propertiesByPath := collectUnknownArgumentsByPath(root)
	if len(propertiesByPath) == 0 {
		return ""
	}

	paths := make([]string, 0, len(propertiesByPath))
	for path := range propertiesByPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	parts := make([]string, 0, len(paths))
	hasTaskID := false
	for _, path := range paths {
		properties := make([]string, 0, len(propertiesByPath[path]))
		for property := range propertiesByPath[path] {
			properties = append(properties, property)
			if property == mcpKeyTaskID {
				hasTaskID = true
			}
		}
		sort.Strings(properties)
		quoted := make([]string, len(properties))
		for i, property := range properties {
			quoted[i] = strconv.Quote(property)
		}
		part := strings.Join(quoted, ", ")
		if len(paths) > 1 || path != "$" || primaryPath != "$" {
			part += " at " + path
		}
		parts = append(parts, part)
	}

	detail := "; unknown arguments: " + strings.Join(parts, "; ")
	if sessionBoundTaskTools[toolName] && hasTaskID {
		detail += " " + sessionBoundTaskRule
	}
	return detail
}

func collectUnknownArgumentsByPath(root *jsonschema.ValidationError) map[string]map[string]struct{} {
	propertiesByPath := make(map[string]map[string]struct{})
	collectUnknownArguments(root, propertiesByPath)
	return propertiesByPath
}

func collectUnknownArguments(
	failure *jsonschema.ValidationError,
	propertiesByPath map[string]map[string]struct{},
) {
	if failure == nil {
		return
	}
	additional, ok := failure.ErrorKind.(*kind.AdditionalProperties)
	if ok && len(additional.Properties) > 0 {
		path := validationInstancePath(failure.InstanceLocation)
		properties := propertiesByPath[path]
		if properties == nil {
			properties = make(map[string]struct{}, len(additional.Properties))
			propertiesByPath[path] = properties
		}
		for _, property := range additional.Properties {
			properties[property] = struct{}{}
		}
	}
	for _, cause := range failure.Causes {
		collectUnknownArguments(cause, propertiesByPath)
	}
}

func missingRequiredProperties(root *jsonschema.ValidationError, primaryPath string) string {
	propertiesByPath := collectMissingRequiredByPath(root)
	if len(propertiesByPath) == 0 {
		return ""
	}

	paths := make([]string, 0, len(propertiesByPath))
	for path := range propertiesByPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	parts := make([]string, 0, len(paths))
	for _, path := range paths {
		missing := make([]string, 0, len(propertiesByPath[path]))
		for property := range propertiesByPath[path] {
			missing = append(missing, strconv.Quote(property))
		}
		sort.Strings(missing)
		part := strings.Join(missing, ", ")
		if len(paths) > 1 || path != "$" || primaryPath != "$" {
			part += " at " + path
		}
		parts = append(parts, part)
	}
	return "; missing: " + strings.Join(parts, "; ")
}

func collectMissingRequiredByPath(root *jsonschema.ValidationError) map[string]map[string]struct{} {
	propertiesByPath := make(map[string]map[string]struct{})
	collectMissingRequired(root, propertiesByPath)
	return propertiesByPath
}

func collectMissingRequired(
	failure *jsonschema.ValidationError,
	propertiesByPath map[string]map[string]struct{},
) {
	if failure == nil {
		return
	}
	required, ok := failure.ErrorKind.(*kind.Required)
	if ok && len(required.Missing) > 0 {
		path := validationInstancePath(failure.InstanceLocation)
		properties := propertiesByPath[path]
		if properties == nil {
			properties = make(map[string]struct{}, len(required.Missing))
			propertiesByPath[path] = properties
		}
		for _, property := range required.Missing {
			properties[property] = struct{}{}
		}
	}
	for _, cause := range failure.Causes {
		collectMissingRequired(cause, propertiesByPath)
	}
}

func firstKeywordFailure(err *jsonschema.ValidationError) (*jsonschema.ValidationError, string) {
	if err == nil {
		return nil, ""
	}
	if err.ErrorKind != nil {
		keywordPath := err.ErrorKind.KeywordPath()
		if len(keywordPath) > 0 && keywordPath[0] != "" {
			return err, keywordPath[0]
		}
	}
	for _, cause := range err.Causes {
		if failure, keyword := firstKeywordFailure(cause); failure != nil {
			return failure, keyword
		}
	}
	return nil, ""
}

func validationInstancePath(tokens []string) string {
	if len(tokens) == 0 {
		return "$"
	}

	var path strings.Builder
	for _, token := range tokens {
		path.WriteByte('/')
		token = strings.ReplaceAll(token, "~", "~0")
		path.WriteString(strings.ReplaceAll(token, "/", "~1"))
	}
	return path.String()
}

func normalizeToolArguments(toolName string, arguments any) (any, error) {
	if toolName != "create_task_kandev" {
		return arguments, nil
	}
	args, ok := arguments.(map[string]any)
	if !ok {
		return arguments, nil
	}
	_, hasPrompt := args["prompt"]
	description, hasDescription := args["description"]
	if hasPrompt && hasDescription {
		return nil, fmt.Errorf("invalid arguments for %s: provide either prompt or description, not both", toolName)
	}
	if !hasDescription {
		return args, nil
	}

	normalized := make(map[string]any, len(args))
	for key, value := range args {
		normalized[key] = value
	}
	normalized["prompt"] = description
	delete(normalized, "description")
	return normalized, nil
}

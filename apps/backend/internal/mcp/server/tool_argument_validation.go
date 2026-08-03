package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

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
	return fmt.Errorf("invalid arguments for %s: validation failed at %s (keyword: %s%s)",
		toolName, validationInstancePath(failure.InstanceLocation), keyword, missingRequiredProperties(failure))
}

func missingRequiredProperties(err *jsonschema.ValidationError) string {
	required, ok := err.ErrorKind.(*kind.Required)
	if !ok || len(required.Missing) == 0 {
		return ""
	}

	missing := make([]string, len(required.Missing))
	for i, property := range required.Missing {
		missing[i] = strconv.Quote(property)
	}
	sort.Strings(missing)
	return "; missing: " + strings.Join(missing, ", ")
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

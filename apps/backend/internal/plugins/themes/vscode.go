package themes

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/tailscale/hujson"
)

type Finding struct {
	Kind    string `json:"kind"`
	Field   string `json:"field"`
	Message string `json:"message"`
}
type Report struct {
	Mapped       int       `json:"mapped"`
	Approximated int       `json:"approximated"`
	Ignored      int       `json:"ignored"`
	Fallback     int       `json:"fallback"`
	Findings     []Finding `json:"findings"`
}
type Import struct {
	Name       string      `json:"name"`
	ID         string      `json:"id"`
	Definition *Definition `json:"definition"`
	Report     Report      `json:"report"`
}

func ConvertVSCode(data []byte, base string) (*Import, error) {
	root, base, err := decodeVSCode(data, base)
	if err != nil {
		return nil, err
	}
	d := &Definition{SchemaVersion: 1, Base: base, UI: map[string]string{}, Editor: map[string]string{}, Terminal: map[string]string{}, Syntax: map[string]Style{}, Semantic: map[string]Style{}}
	result := &Import{Name: "Imported theme", Definition: d, Report: Report{Findings: []Finding{}}}
	if name, ok := root["name"].(string); ok && strings.TrimSpace(name) != "" {
		result.Name = name
	}
	if len(result.Name) > 128 {
		return nil, fmt.Errorf("theme_name: maximum 128 bytes")
	}
	if err = convertTokens(root["tokenColors"], result); err != nil {
		return nil, err
	}
	if err = convertColors(root["colors"], result); err != nil {
		return nil, err
	}
	if err = convertSemantic(root["semanticTokenColors"], result); err != nil {
		return nil, err
	}
	canonical, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	result.Definition, err = Parse(canonical, base)
	if err != nil {
		return nil, err
	}
	result.Report.Fallback = CountFallback(result.Definition)
	sum := sha256.Sum256(append(append([]byte(nil), data...), []byte("\x00"+base)...))
	result.ID = "imported-" + hex.EncodeToString(sum[:16])
	return result, nil
}

func decodeVSCode(data []byte, base string) (map[string]any, string, error) {
	if len(data) > MaxDefinitionBytes {
		return nil, "", fmt.Errorf("theme_too_large")
	}
	standard, err := hujson.Standardize(data)
	if err != nil {
		return nil, "", fmt.Errorf("theme_jsonc: %w", err)
	}
	value, err := strictJSON(standard)
	if err != nil {
		return nil, "", err
	}
	root, ok := value.(map[string]any)
	if !ok {
		return nil, "", fmt.Errorf("theme_json: object required")
	}
	if _, ok := root["include"]; ok {
		return nil, "", fmt.Errorf("theme_include: inheritance is unsupported")
	}
	if declared, ok := root["type"]; ok {
		s, valid := declared.(string)
		if !valid || (s != BaseLight && s != BaseDark) {
			return nil, "", fmt.Errorf("theme_base: unsupported type")
		}
		if base != "" && base != s {
			return nil, "", fmt.Errorf("theme_base: type disagrees with selected base")
		}
		base = s
	}
	if base != BaseLight && base != BaseDark {
		return nil, "", fmt.Errorf("theme_base_required: choose light or dark")
	}

	return root, base, nil
}

func (r *Report) add(kind, field, message string) {
	switch kind {
	case "mapped":
		r.Mapped++
	case "approximate":
		r.Approximated++
	case "ignored":
		r.Ignored++
	}
	if len(r.Findings) < 256 {
		r.Findings = append(r.Findings, Finding{Kind: kind, Field: field, Message: message})
	}
}

func convertColors(value any, result *Import) error {
	if value == nil {
		return nil
	}
	colors, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("theme_colors: object required")
	}
	for _, key := range sortedKeys(colors) {
		targets, ok := vscodeColors[key]
		if !ok {
			result.Report.add("ignored", "colors."+key, "Unsupported VS Code color")
			continue
		}
		color, ok := colors[key].(string)
		if !ok {
			return fmt.Errorf("theme_color: %s requires a color string", key)
		}
		for _, target := range targets {
			group, role, _ := strings.Cut(target, ".")
			switch group {
			case "ui":
				result.Definition.UI[role] = color
			case "editor":
				result.Definition.Editor[role] = color
			case "terminal":
				result.Definition.Terminal[role] = color
			}
		}
		result.Report.add("mapped", "colors."+key, "Mapped to portable colors")
	}
	return nil
}

func convertTokens(value any, result *Import) error {
	if value == nil {
		return nil
	}
	rules, ok := value.([]any)
	if !ok || len(rules) > 1024 {
		return fmt.Errorf("theme_tokens: bounded tokenColors array required; file references are unsupported")
	}
	for i, raw := range rules {
		rule, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("theme_tokens: invalid rule %d", i)
		}
		settings, err := json.Marshal(rule["settings"])
		if err != nil {
			return err
		}
		var style Style
		if err = json.Unmarshal(settings, &style); err != nil {
			return fmt.Errorf("theme_tokens: invalid settings")
		}
		if err = validateTokenSettings(rule["settings"]); err != nil {
			return err
		}
		scope, scoped := rule["scope"]
		if text, ok := scope.(string); ok && strings.TrimSpace(text) == "" {
			scoped = false
		}
		if !scoped {
			if err := convertTokenDefaults(style, result); err != nil {
				return err
			}
			continue
		}
		scopes, err := tokenScopes(scope)
		if err != nil {
			return err
		}
		convertScopedToken(scopes, style, result)
	}
	return nil
}

func convertScopedToken(scopes []string, style Style, result *Import) {
	result.Definition.Textmate = append(result.Definition.Textmate, TokenRule{Scope: scopes, Settings: style})
	for _, scope := range scopes {
		mapped := false
		for alias, role := range scopeAliases {
			if scope == alias || strings.HasPrefix(scope, alias+".") {
				result.Definition.Syntax[role] = style
				mapped = true
			}
		}
		if mapped {
			result.Report.add("mapped", scope, "Portable syntax approximation; TextMate rule retained")
		} else {
			result.Report.add("approximate", scope, "TextMate retained; Monaco and CodeMirror use portable fallback")
		}
	}
}

func validateTokenSettings(value any) error {
	settings, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("theme_tokens: settings object required")
	}
	for key := range settings {
		if key != "foreground" && key != "background" && key != "fontStyle" {
			return fmt.Errorf("theme_tokens: unsupported setting %s", key)
		}
	}
	probe, err := json.Marshal(map[string]any{"schema_version": 1, "textmate": []any{map[string]any{"scope": "comment", "settings": value}}})
	if err != nil {
		return err
	}
	_, err = Parse(probe, BaseDark)
	return err
}

func convertTokenDefaults(style Style, result *Import) error {
	colors := map[string]string{}
	if style.Foreground != "" {
		colors["foreground"] = style.Foreground
	}
	if style.Background != "" {
		colors["background"] = style.Background
	}
	probe, err := json.Marshal(Definition{SchemaVersion: 1, Editor: colors})
	if err != nil {
		return err
	}
	if _, err = Parse(probe, result.Definition.Base); err != nil {
		return err
	}
	for _, role := range sortedKeys(colors) {
		color := colors[role]
		result.Definition.Editor[role] = color
		result.Report.add("mapped", "tokenColors.defaults."+role, "Mapped default editor color")
	}
	if style.FontStyle != "" {
		result.Report.add("ignored", "tokenColors.defaults.fontStyle", "Unscoped font style is unsupported")
	}
	return nil
}

func tokenScopes(value any) ([]string, error) {
	var scopes []string
	switch v := value.(type) {
	case string:
		scopes = strings.Split(v, ",")
	case []any:
		for _, scope := range v {
			s, ok := scope.(string)
			if !ok {
				return nil, fmt.Errorf("theme_scope: string required")
			}
			scopes = append(scopes, s)
		}
	default:
		return nil, fmt.Errorf("theme_scope: string or array required")
	}
	for i, s := range scopes {
		scopes[i] = strings.TrimSpace(s)
	}
	return scopes, nil
}

func convertSemantic(value any, result *Import) error {
	if value == nil {
		return nil
	}
	rules, ok := value.(map[string]any)
	if !ok || len(rules) > 256 {
		return fmt.Errorf("theme_semantic: bounded object required")
	}
	for _, key := range sortedKeys(rules) {
		style, err := semanticStyle(rules[key])
		if err != nil {
			return err
		}
		probe := Definition{SchemaVersion: 1, Semantic: map[string]Style{key: style}}
		data, _ := json.Marshal(probe)
		if _, err = Parse(data, result.Definition.Base); err != nil {
			// Validate style independently so a supported selector never hides an invalid color.
			probe.Semantic = map[string]Style{"variable": style}
			data, _ = json.Marshal(probe)
			if _, styleErr := Parse(data, result.Definition.Base); styleErr != nil {
				return styleErr
			}
			result.Report.add("ignored", "semanticTokenColors."+key, "Unsupported semantic selector")
			continue
		}
		result.Definition.Semantic[key] = style
		result.Report.add("mapped", "semanticTokenColors."+key, "Mapped semantic token rule")
	}
	return nil
}

func semanticStyle(value any) (Style, error) {
	if color, ok := value.(string); ok {
		return Style{Foreground: color}, nil
	}
	fields, ok := value.(map[string]any)
	if !ok {
		return Style{}, fmt.Errorf("theme_semantic: color or style required")
	}
	var style Style
	var flags []string
	for _, key := range sortedKeys(fields) {
		switch key {
		case "foreground":
			color, ok := fields[key].(string)
			if !ok {
				return style, fmt.Errorf("theme_semantic: foreground string required")
			}
			style.Foreground = color
		case "bold", "italic", "underline", "strikethrough":
			flag, ok := fields[key].(bool)
			if !ok {
				return style, fmt.Errorf("theme_semantic: boolean style required")
			}
			if flag {
				flags = append(flags, key)
			}
		default:
			return style, fmt.Errorf("theme_semantic: unsupported style field %s", key)
		}
	}
	style.FontStyle = strings.Join(flags, " ")
	return style, nil
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func CountFallback(d *Definition) int {
	var schema struct {
		Properties map[string]struct {
			Properties map[string]any `json:"properties"`
		} `json:"properties"`
	}
	_ = json.Unmarshal(Schema, &schema)
	return len(schema.Properties["ui"].Properties) + len(schema.Properties["editor"].Properties) + len(schema.Properties["terminal"].Properties) + len(schema.Properties["syntax"].Properties) - len(d.UI) - len(d.Editor) - len(d.Terminal) - len(d.Syntax)
}

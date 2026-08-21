package automation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"

	"gopkg.in/yaml.v3"
)

// yamlStrTag is the explicit YAML 1.1 string tag, used wherever a scalar must
// stay a string on emission rather than retyping to bool/int/etc. (AC-23).
const yamlStrTag = "!!str"

// decodeJSONWithNumbers decodes JSON bytes into Go values using json.Number for
// numeric literals rather than float64, so AC-41's byte-for-byte numeric fidelity
// (e.g. 18446744073709551616, which overflows both int64 and uint64, or 1e400, which
// overflows float64) survives the decode step untouched.
//
// json.Decoder.Decode only consumes one JSON value and stops; unlike
// json.Unmarshal it does not reject trailing bytes after that value. A
// trailing-data check is required so `{"a":1} garbage` is treated as
// invalid JSON (AC-39) rather than silently accepted as `{"a":1}`.
func decodeJSONWithNumbers(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("automation: trailing data after JSON value")
	}
	return v, nil
}

// jsonToYAMLNode converts a JSON value decoded with UseNumber into a *yaml.Node,
// following the per-JSON-type table AC-8 and AC-41 require:
//
//   - object (map[string]any)  -> MappingNode, keys sorted byte-wise ascending, at
//     every nesting depth — never yaml.v3's own map-key sorter, which orders "v1",
//     "v2", "v10" instead of the byte-wise "v1", "v10", "v2".
//   - array ([]any)            -> SequenceNode, stored order preserved, never sorted.
//   - string                   -> ScalarNode with an explicit !!str tag, so a string
//     like "true" or "12" is quoted on emission and re-parses as a string rather than
//     retyping to bool/int (AC-23).
//   - json.Number              -> ScalarNode with Tag left unset and Value set to the
//     original lexeme. Tag must stay unset: setting !!int/!!float explicitly makes
//     the encoder compare against its own re-resolution of Value and print the tag
//     whenever they disagree, which happens for values outside int64/uint64/float64
//     range. Never convert through int64/float64 — that is exactly the rounding
//     AC-41 forbids.
//   - bool                     -> ScalarNode, Tag !!bool.
//   - nil                      -> ScalarNode, Tag !!null, Value "null".
func jsonToYAMLNode(v any) (*yaml.Node, error) {
	switch val := v.(type) {
	case map[string]any:
		return jsonObjectToYAMLNode(val)
	case []any:
		return jsonArrayToYAMLNode(val)
	case string:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: yamlStrTag, Value: val}, nil
	case json.Number:
		return &yaml.Node{Kind: yaml.ScalarNode, Value: val.String()}, nil
	case bool:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(val)}, nil
	case nil:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}, nil
	default:
		return nil, fmt.Errorf("automation: unsupported JSON value type %T for YAML export", v)
	}
}

func jsonObjectToYAMLNode(obj map[string]any) (*yaml.Node, error) {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys) // byte-wise ascending; Go string comparison is byte-wise UTF-8.

	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, k := range keys {
		valueNode, err := jsonToYAMLNode(obj[k])
		if err != nil {
			return nil, err
		}
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: yamlStrTag, Value: k},
			valueNode,
		)
	}
	return node, nil
}

func jsonArrayToYAMLNode(arr []any) (*yaml.Node, error) {
	node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, item := range arr {
		itemNode, err := jsonToYAMLNode(item)
		if err != nil {
			return nil, err
		}
		node.Content = append(node.Content, itemNode)
	}
	return node, nil
}

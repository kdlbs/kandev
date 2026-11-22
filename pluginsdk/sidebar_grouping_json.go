package pluginsdk

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// Canonical wire field spellings, in decode-index order.
const (
	sidebarFieldRevision    = "revision"
	sidebarFieldGroups      = "groups"
	sidebarFieldMemberships = "memberships"
	sidebarFieldLabel       = "label"
	sidebarFieldTaskID      = "task_id"
	sidebarFieldGroupID     = "group_id"
)

var (
	sidebarSnapshotFields   = []string{"version", sidebarFieldRevision, "complete", sidebarFieldGroups, sidebarFieldMemberships}
	sidebarValueFields      = []string{"id", sidebarFieldLabel, "color"}
	sidebarMembershipFields = []string{sidebarFieldTaskID, sidebarFieldGroupID}
)

func decodeSidebarGroupingJSON(body []byte) (SidebarGroupingSnapshot, error) {
	var snapshot SidebarGroupingSnapshot
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	err := decodeSidebarObject(decoder, sidebarSnapshotFields, func(index int) error {
		switch index {
		case 0:
			return decoder.Decode(&snapshot.Version)
		case 1:
			return decoder.Decode(&snapshot.Revision)
		case 2:
			return decoder.Decode(&snapshot.Complete)
		case 3:
			var err error
			snapshot.Groups, err = decodeSidebarArray(decoder, MaxSidebarGroupingValues, decodeSidebarValue)
			return err
		default:
			var err error
			snapshot.Memberships, err = decodeSidebarArray(decoder, MaxSidebarGroupingMemberships, decodeSidebarMembership)
			return err
		}
	})
	if err == nil {
		if _, trailing := decoder.Token(); trailing != io.EOF {
			err = errInvalidSidebarGroupingSnapshot
		}
	}
	return snapshot, err
}

// Recognized wire fields have one canonical spelling and occur at most once.
// Unknown extension fields do not participate in classification.
func decodeSidebarObject(decoder *json.Decoder, fields []string, read func(int) error) error {
	if err := sidebarJSONDelimiter(decoder, '{'); err != nil {
		return err
	}
	var seen uint8
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		name, ok := token.(string)
		if !ok {
			return errInvalidSidebarGroupingSnapshot
		}
		index := -1
		for i, field := range fields {
			if !strings.EqualFold(name, field) {
				continue
			}
			if name != field || seen&(1<<i) != 0 {
				return errInvalidSidebarGroupingSnapshot
			}
			index = i
			seen |= 1 << i
			break
		}
		if index < 0 {
			err = skipSidebarJSONValue(decoder)
		} else {
			err = read(index)
		}
		if err != nil {
			return err
		}
	}
	return sidebarJSONDelimiter(decoder, '}')
}

func decodeSidebarArray[T any](decoder *json.Decoder, limit int, read func(*json.Decoder, *T) error) ([]T, error) {
	if err := sidebarJSONDelimiter(decoder, '['); err != nil {
		return nil, err
	}
	values := make([]T, 0)
	for decoder.More() {
		if len(values) == limit {
			return nil, errInvalidSidebarGroupingSnapshot
		}
		var zero T
		values = append(values, zero)
		if err := read(decoder, &values[len(values)-1]); err != nil {
			return nil, err
		}
	}
	return values, sidebarJSONDelimiter(decoder, ']')
}

func decodeSidebarValue(decoder *json.Decoder, value *SidebarGroupingValue) error {
	return decodeSidebarObject(decoder, sidebarValueFields, func(index int) error {
		switch index {
		case 0:
			return decoder.Decode(&value.ID)
		case 1:
			return decoder.Decode(&value.Label)
		default:
			return decoder.Decode(&value.Color)
		}
	})
}

func decodeSidebarMembership(decoder *json.Decoder, value *SidebarGroupingMembership) error {
	return decodeSidebarObject(decoder, sidebarMembershipFields, func(index int) error {
		if index == 0 {
			return decoder.Decode(&value.TaskID)
		}
		return decoder.Decode(&value.GroupID)
	})
}

func sidebarJSONDelimiter(decoder *json.Decoder, want json.Delim) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token != want {
		return errInvalidSidebarGroupingSnapshot
	}
	return nil
}

func skipSidebarJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token != json.Delim('{') && token != json.Delim('[') {
		return nil
	}
	for depth := 1; depth > 0; {
		token, err = decoder.Token()
		if err != nil {
			return err
		}
		switch token {
		case json.Delim('{'), json.Delim('['):
			depth++
		case json.Delim('}'), json.Delim(']'):
			depth--
		}
	}
	return nil
}

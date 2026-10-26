package updates

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path/filepath"
)

func launchdServiceShell(data []byte, metadata *serviceInstallMetadata) (string, error) {
	arguments, err := launchdProgramArguments(data)
	if err != nil {
		return "", err
	}
	if legacyLaunchdServiceInvocation(arguments, metadata) {
		return "", nil
	}
	if len(arguments) < 1 || !filepath.IsAbs(arguments[0]) || filepath.Base(arguments[0]) != "kandev" {
		return "", errors.New("ProgramArguments must start with the absolute kandev executable")
	}
	if len(arguments) == 2 && arguments[1] == "--headless" {
		return "", nil
	}
	if len(arguments) == 5 && arguments[1] == "service" && arguments[2] == "run" && arguments[3] == "--shell" {
		return arguments[4], nil
	}
	return "", errors.New("ProgramArguments is not a recognized Kandev service invocation")
}

func legacyLaunchdServiceInvocation(arguments []string, metadata *serviceInstallMetadata) bool {
	if metadata == nil || metadata.NodePath == "" || metadata.CLIEntry == "" ||
		!filepath.IsAbs(metadata.NodePath) || !filepath.IsAbs(metadata.CLIEntry) || len(arguments) < 2 {
		return false
	}
	if arguments[0] != metadata.NodePath || arguments[1] != metadata.CLIEntry {
		return false
	}
	return len(arguments) == 2 || (len(arguments) == 3 && arguments[2] == "--headless")
}

func launchdProgramArguments(data []byte) ([]string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var key string
	var arguments []string
	found := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse plist XML: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local == "key" {
			if err := decoder.DecodeElement(&key, &start); err != nil {
				return nil, fmt.Errorf("parse plist key: %w", err)
			}
			continue
		}
		if key != "ProgramArguments" {
			key = ""
			continue
		}
		if start.Name.Local != "array" || found {
			return nil, errors.New("ProgramArguments must be one array")
		}
		arguments, err = launchdStringArray(decoder, start)
		if err != nil {
			return nil, fmt.Errorf("parse ProgramArguments: %w", err)
		}
		found = true
		key = ""
	}
	if !found || len(arguments) == 0 {
		return nil, errors.New("ProgramArguments array is missing or empty")
	}
	return arguments, nil
}

func launchdStringArray(decoder *xml.Decoder, array xml.StartElement) ([]string, error) {
	var values []string
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		switch element := token.(type) {
		case xml.EndElement:
			if element.Name == array.Name {
				return values, nil
			}
		case xml.StartElement:
			if element.Name.Local != "string" {
				return nil, fmt.Errorf("ProgramArguments contains unsupported <%s> value", element.Name.Local)
			}
			var value string
			if err := decoder.DecodeElement(&value, &element); err != nil {
				return nil, err
			}
			values = append(values, value)
		}
	}
}

package launcher

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const rootServiceUser = "root"

func resolveSystemServiceUser(args serviceArgs, servicePath, manager string) (string, error) {
	if !args.System {
		return "", nil
	}
	if args.RunAs != "" {
		if err := validateSystemServiceUser(args.RunAs); err != nil {
			return "", err
		}
		return args.RunAs, nil
	}

	userName, found, err := readManagedSystemServiceUser(servicePath, manager)
	if err != nil {
		return "", err
	}
	if found {
		if err := validateSystemServiceUser(userName); err != nil {
			return "", fmt.Errorf("preserve service account %q from %s: %w", userName, servicePath, err)
		}
		return userName, nil
	}

	if sudoUser := strings.TrimSpace(os.Getenv("SUDO_USER")); sudoUser != "" && sudoUser != rootServiceUser {
		if err := validateSystemServiceUser(sudoUser); err != nil {
			return "", err
		}
		return sudoUser, nil
	}
	return "", fmt.Errorf(
		"cannot infer the system service account; run `kandev service install --system --run-as <user>` (use --run-as " + rootServiceUser + " to choose root explicitly)",
	)
}

func validateSystemServiceUser(userName string) error {
	trimmed := strings.TrimSpace(userName)
	if trimmed == "" || trimmed != userName || strings.ContainsAny(userName, " \t\r\n") {
		return errors.New("system service account must be a non-empty username")
	}
	if _, _, err := lookupNativeServiceOwner(userName); err != nil {
		return fmt.Errorf("resolve system service user %q: %w", userName, err)
	}
	return nil
}

func readManagedSystemServiceUser(servicePath, manager string) (string, bool, error) {
	info, err := os.Lstat(servicePath)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("inspect existing service definition %q: %w", servicePath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", false, fmt.Errorf("managed service definition %q must not be a symlink", servicePath)
	}
	if !info.Mode().IsRegular() {
		return "", false, fmt.Errorf("managed service definition %q is not a regular file", servicePath)
	}
	data, err := os.ReadFile(servicePath)
	if err != nil {
		return "", false, fmt.Errorf("read existing service definition %q: %w", servicePath, err)
	}
	if !bytes.Contains(data, []byte(managedMarker)) {
		return "", false, nil
	}

	switch manager {
	case nativeServiceManagerSystemd:
		return parseManagedSystemdUser(data, servicePath)
	case nativeServiceManagerLaunchd:
		return parseManagedLaunchdUser(data, servicePath)
	default:
		return "", false, fmt.Errorf("unsupported service manager %q", manager)
	}
}

func parseManagedSystemdUser(data []byte, servicePath string) (string, bool, error) {
	var userName string
	seen := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !strings.HasPrefix(line, "User=") {
			continue
		}
		if seen {
			return "", false, fmt.Errorf("managed systemd definition %q contains multiple User= entries", servicePath)
		}
		seen = true
		userName = strings.TrimSpace(strings.TrimPrefix(line, "User="))
		if userName == "" {
			return "", false, fmt.Errorf("managed systemd definition %q has an empty User= entry", servicePath)
		}
	}
	if !seen {
		return rootServiceUser, true, nil
	}
	return userName, true, nil
}

func parseManagedLaunchdUser(data []byte, servicePath string) (string, bool, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	userKey := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", false, fmt.Errorf("parse managed launchd definition %q: %w", servicePath, err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "key":
			var key string
			if err := decoder.DecodeElement(&key, &start); err != nil {
				return "", false, fmt.Errorf("parse managed launchd key %q: %w", servicePath, err)
			}
			userKey = key == "UserName"
		case "string":
			if !userKey {
				continue
			}
			var userName string
			if err := decoder.DecodeElement(&userName, &start); err != nil {
				return "", false, fmt.Errorf("parse managed launchd user %q: %w", servicePath, err)
			}
			if strings.TrimSpace(userName) == "" {
				return "", false, fmt.Errorf("managed launchd definition %q has an empty UserName", servicePath)
			}
			return strings.TrimSpace(userName), true, nil
		default:
			if userKey {
				return "", false, fmt.Errorf("managed launchd definition %q has a UserName without a string value", servicePath)
			}
			userKey = false
		}
	}
	return rootServiceUser, true, nil
}

func validateSystemServiceHomeOwner(homeDir, userName string) error {
	if userName == "" {
		return nil
	}
	info, err := os.Lstat(homeDir)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("system service home %q does not exist; pre-create it and chown it to %q before installing", homeDir, userName)
	}
	if err != nil {
		return fmt.Errorf("inspect system service home %q: %w", homeDir, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("system service home %q must not be a symlink", homeDir)
	}
	if !info.IsDir() {
		return fmt.Errorf("system service home %q is not a directory", homeDir)
	}
	expectedUID, _, err := lookupNativeServiceOwner(userName)
	if err != nil {
		return fmt.Errorf("resolve owner for system service home %q: %w", homeDir, err)
	}
	actualUID, err := nativePathOwnerUID(homeDir)
	if err != nil {
		return fmt.Errorf("inspect owner of system service home %q: %w", homeDir, err)
	}
	if actualUID != expectedUID {
		return fmt.Errorf(
			"system service home %q is owned by uid %d but service account %q uses uid %d; preserve the existing account or reconcile ownership explicitly before reinstalling",
			homeDir, actualUID, userName, expectedUID,
		)
	}
	return nil
}

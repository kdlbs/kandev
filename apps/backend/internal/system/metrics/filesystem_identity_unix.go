//go:build !windows

package metrics

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
)

func FilesystemIdentity(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	value := reflect.ValueOf(info.Sys())
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	device := value.FieldByName("Dev")
	if !device.IsValid() {
		return "", fmt.Errorf("filesystem identity unavailable for %q", path)
	}
	switch device.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return "device:" + strconv.FormatUint(device.Uint(), 10), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "device:" + strconv.FormatInt(device.Int(), 10), nil
	default:
		return "", fmt.Errorf("filesystem identity unavailable for %q", path)
	}
}

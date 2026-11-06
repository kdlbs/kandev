package models

import (
	"bytes"
	"encoding/gob"
	"reflect"
)

type usageMeasurementValue SessionUsageMeasurement

type usageMeasurementEncoding struct {
	Value   usageMeasurementValue
	Present []string
}

// GobEncode retains nullable measured zeroes. Gob's default pointer flattening
// otherwise makes a pointer to zero indistinguishable from an absent amount.
func (m SessionUsageMeasurement) GobEncode() ([]byte, error) {
	value := reflect.ValueOf(m)
	encoded := usageMeasurementEncoding{Value: usageMeasurementValue(m)}
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		if field.Kind() == reflect.Pointer && !field.IsNil() {
			encoded.Present = append(encoded.Present, value.Type().Field(i).Name)
		}
	}
	var buffer bytes.Buffer
	err := gob.NewEncoder(&buffer).Encode(encoded)
	return buffer.Bytes(), err
}

func (m *SessionUsageMeasurement) GobDecode(data []byte) error {
	var encoded usageMeasurementEncoding
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&encoded); err != nil {
		return err
	}
	*m = SessionUsageMeasurement(encoded.Value)
	value := reflect.ValueOf(m).Elem()
	for _, name := range encoded.Present {
		field := value.FieldByName(name)
		if field.IsValid() && field.Kind() == reflect.Pointer && field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
	}
	return nil
}

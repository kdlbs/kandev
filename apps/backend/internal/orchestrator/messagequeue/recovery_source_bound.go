package messagequeue

import "reflect"

// Preflight in-memory values before JSON encoding or deep copying. Stored JSON
// payloads use the SQL byte-length preflight; memory must also reject large
// strings, excessive nesting and cyclic containers without allocating their
// encoding or recursively copying them.
func recoveryValueBound(value reflect.Value, remaining *int, depth int) error {
	if !value.IsValid() {
		return nil
	}
	*remaining -= 1
	if *remaining < 0 || depth > 32 {
		return ErrRecoveryLimit
	}
	switch value.Kind() {
	case reflect.String:
		*remaining -= value.Len()
	case reflect.Interface, reflect.Pointer:
		if !value.IsNil() {
			return recoveryValueBound(value.Elem(), remaining, depth+1)
		}
	case reflect.Map:
		return recoveryMapBound(value, remaining, depth)
	case reflect.Slice, reflect.Array:
		return recoverySliceBound(value, remaining, depth)
	case reflect.Struct:
		return recoveryStructBound(value, remaining, depth)

	}
	if *remaining < 0 {
		return ErrRecoveryLimit
	}
	return nil
}

func recoveryMapBound(value reflect.Value, remaining *int, depth int) error {
	if value.Len() > *remaining {
		return ErrRecoveryLimit
	}
	iterator := value.MapRange()
	for iterator.Next() {
		if err := recoveryValueBound(iterator.Key(), remaining, depth+1); err != nil {
			return err
		}
		if err := recoveryValueBound(iterator.Value(), remaining, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func recoverySliceBound(value reflect.Value, remaining *int, depth int) error {
	if value.Len() > *remaining {
		return ErrRecoveryLimit
	}
	for i := range value.Len() {
		if err := recoveryValueBound(value.Index(i), remaining, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func recoveryStructBound(value reflect.Value, remaining *int, depth int) error {
	for i := range value.NumField() {
		if value.Type().Field(i).IsExported() {
			if err := recoveryValueBound(value.Field(i), remaining, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

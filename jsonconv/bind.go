package jsonconv

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"typeconverter/converter"
)

func (s *Service) bind(src any, dest any) error {
	v := reflect.ValueOf(dest)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return fmt.Errorf("dest must be a non-nil pointer")
	}
	return s.bindValue(src, v.Elem())
}

func (s *Service) bindValue(src any, dest reflect.Value) error {
	if !dest.IsValid() {
		return fmt.Errorf("invalid destination")
	}
	if src == nil {
		dest.Set(reflect.Zero(dest.Type()))
		return nil
	}
	if dest.Kind() == reflect.Pointer {
		if dest.IsNil() {
			dest.Set(reflect.New(dest.Type().Elem()))
		}
		return s.bindValue(src, dest.Elem())
	}
	if dest.Type() == reflect.TypeOf(json.RawMessage(nil)) {
		raw, err := json.Marshal(src)
		if err != nil {
			return err
		}
		dest.Set(reflect.ValueOf(json.RawMessage(raw)))
		return nil
	}

	switch dest.Kind() {
	case reflect.Struct:
		return s.bindStruct(src, dest)
	case reflect.Map:
		return s.bindMap(src, dest)
	case reflect.Slice, reflect.Array:
		return s.bindSlice(src, dest)
	case reflect.Interface:
		if dest.NumMethod() != 0 {
			return fmt.Errorf("cannot bind JSON into %s", dest.Type())
		}
		dest.Set(reflect.ValueOf(src))
		return nil
	default:
		return s.bindScalar(src, dest)
	}
}

func (s *Service) bindStruct(src any, dest reflect.Value) error {
	obj, ok := src.(map[string]any)
	if !ok {
		return fmt.Errorf("cannot bind %T into struct %s", src, dest.Type())
	}

	fields, err := collectFields(dest)
	if err != nil {
		return err
	}

	used := make(map[string]bool)
	var anyField *boundField

	for i := range fields {
		f := &fields[i]
		if f.ignore {
			continue
		}
		if f.any {
			anyField = f
			continue
		}
		val, key, found := lookup(obj, f)
		if !found {
			continue
		}
		used[key] = true
		if err := s.bindValue(val, f.value); err != nil {
			return fmt.Errorf("field %s: %w", f.fieldName, err)
		}
	}

	if anyField != nil {
		leftover := leftoverObject(obj, used)
		if len(leftover) == 0 {
			return nil
		}
		if err := s.bindValue(leftover, anyField.value); err != nil {
			return fmt.Errorf("field %s: %w", anyField.fieldName, err)
		}
	}
	return nil
}

func (s *Service) bindMap(src any, dest reflect.Value) error {
	obj, ok := src.(map[string]any)
	if !ok {
		return fmt.Errorf("cannot bind %T into map %s", src, dest.Type())
	}
	if dest.IsNil() {
		dest.Set(reflect.MakeMap(dest.Type()))
	}
	if dest.Type().Key().Kind() != reflect.String {
		return fmt.Errorf("map key must be string, got %s", dest.Type().Key())
	}

	elemType := dest.Type().Elem()
	if elemType == reflect.TypeOf((*any)(nil)).Elem() {
		for k, v := range obj {
			dest.SetMapIndex(reflect.ValueOf(k), reflect.ValueOf(v))
		}
		return nil
	}

	for k, v := range obj {
		elem := reflect.New(elemType).Elem()
		if err := s.bindValue(v, elem); err != nil {
			return fmt.Errorf("key %s: %w", k, err)
		}
		dest.SetMapIndex(reflect.ValueOf(k), elem)
	}
	return nil
}

func (s *Service) bindSlice(src any, dest reflect.Value) error {
	arr, ok := src.([]any)
	if !ok {
		return fmt.Errorf("cannot bind %T into %s", src, dest.Type())
	}

	if dest.Kind() == reflect.Array {
		if len(arr) > dest.Len() {
			return fmt.Errorf("array overflow: have %d values, array length %d", len(arr), dest.Len())
		}
		for i, item := range arr {
			if err := s.bindValue(item, dest.Index(i)); err != nil {
				return err
			}
		}
		return nil
	}

	slice := reflect.MakeSlice(dest.Type(), len(arr), len(arr))
	for i, item := range arr {
		if err := s.bindValue(item, slice.Index(i)); err != nil {
			return err
		}
	}
	dest.Set(slice)
	return nil
}

func (s *Service) bindScalar(src any, dest reflect.Value) error {
	if dest.Kind() == reflect.Pointer {
		if dest.IsNil() {
			dest.Set(reflect.New(dest.Type().Elem()))
		}
		return s.bindScalar(src, dest.Elem())
	}
	if dest.Kind() == reflect.Interface && dest.NumMethod() == 0 {
		dest.Set(reflect.ValueOf(src))
		return nil
	}

	switch dest.Kind() {
	case reflect.String:
		v, err := converter.Convert[string](src)
		if err != nil {
			return err
		}
		dest.SetString(v)
		return nil
	case reflect.Bool:
		v, err := converter.Convert[bool](src)
		if err != nil {
			return err
		}
		dest.SetBool(v)
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v, err := converter.Convert[int64](src)
		if err != nil {
			return err
		}
		dest.SetInt(v)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v, err := converter.Convert[int64](src)
		if err != nil {
			return err
		}
		if v < 0 {
			return fmt.Errorf("cannot convert %v to %s", src, dest.Type())
		}
		dest.SetUint(uint64(v))
		return nil
	case reflect.Float32, reflect.Float64:
		v, err := converter.Convert[float64](src)
		if err != nil {
			return err
		}
		dest.SetFloat(v)
		return nil
	default:
		return fmt.Errorf("unsupported destination type %s", dest.Type())
	}
}

type boundField struct {
	value     reflect.Value
	fieldName string
	name      string
	path      []string
	any       bool
	ignore    bool
}

func collectFields(v reflect.Value) ([]boundField, error) {
	t := v.Type()
	var fields []boundField
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.PkgPath != "" && !sf.Anonymous {
			continue
		}
		fv := v.Field(i)
		if sf.Anonymous && sf.Type.Kind() == reflect.Struct && sf.Tag.Get("json") == "" {
			nested, err := collectFields(fv)
			if err != nil {
				return nil, err
			}
			fields = append(fields, nested...)
			continue
		}
		bf, err := parseField(sf, fv)
		if err != nil {
			return nil, err
		}
		fields = append(fields, bf)
	}
	return fields, nil
}

func parseField(sf reflect.StructField, fv reflect.Value) (boundField, error) {
	bf := boundField{
		value:     fv,
		fieldName: sf.Name,
	}
	tag := sf.Tag.Get("json")
	if tag == "-" {
		bf.ignore = true
		return bf, nil
	}
	if tag == "" {
		return bf, nil
	}

	parts := strings.Split(tag, ",")
	name := parts[0]
	if name != "" {
		if strings.Contains(name, ".") {
			bf.path = strings.Split(name, ".")
		} else {
			bf.name = name
		}
	}
	for _, flag := range parts[1:] {
		switch strings.TrimSpace(flag) {
		case "any":
			bf.any = true
		case "omitempty", "string", "inline":
		default:
			return bf, fmt.Errorf("unknown json flag %q on field %s", flag, sf.Name)
		}
	}
	return bf, nil
}

func lookup(obj map[string]any, f *boundField) (any, string, bool) {
	if len(f.path) > 0 {
		val, ok := lookupPath(obj, f.path)
		return val, f.path[0], ok
	}
	name := f.name
	if name == "" {
		name = f.fieldName
	}
	if val, ok := obj[name]; ok {
		return val, name, true
	}
	for k, v := range obj {
		if strings.EqualFold(k, name) {
			return v, k, true
		}
	}
	return nil, "", false
}

func lookupPath(obj map[string]any, path []string) (any, bool) {
	var current any = obj
	for _, segment := range path {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		val, ok := m[segment]
		if !ok {
			for k, v := range m {
				if strings.EqualFold(k, segment) {
					val = v
					ok = true
					break
				}
			}
		}
		if !ok {
			return nil, false
		}
		current = val
	}
	return current, true
}

func leftoverObject(obj map[string]any, used map[string]bool) map[string]any {
	leftover := make(map[string]any)
	for k, v := range obj {
		if !used[k] {
			leftover[k] = v
		}
	}
	return leftover
}

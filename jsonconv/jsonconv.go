package jsonconv

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// Service converts JSON of any shape into DTO values: structs, slices, maps, or primitives.
type Service struct{}

func NewService() *Service {
	return &Service{}
}

var defaultService = NewService()

// Parse reads JSON into a generic value (map, slice, or primitive) that can
// represent any document structure.
func Parse(data []byte) (any, error) {
	return defaultService.Parse(data)
}

func (s *Service) Parse(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}
	return normalize(value), nil
}

// Decode unmarshals JSON into dest. dest must be a non-nil pointer to a struct,
// map, slice, or primitive. Unknown nested fragments can be captured as
// map[string]any, any, or json.RawMessage fields.
func Decode(data []byte, dest any) error {
	return defaultService.Decode(data, dest)
}

func (s *Service) Decode(data []byte, dest any) error {
	value, err := s.Parse(data)
	if err != nil {
		return err
	}
	return s.bind(value, dest)
}

// DecodeFile reads a JSON file and decodes it into dest.
func DecodeFile(path string, dest any) error {
	return defaultService.DecodeFile(path, dest)
}

func (s *Service) DecodeFile(path string, dest any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read json file: %w", err)
	}
	return s.Decode(data, dest)
}

// DecodeTo unmarshals JSON into a value of type T.
func DecodeTo[T any](data []byte) (T, error) {
	return DecodeToWith[T](defaultService, data)
}

func DecodeToWith[T any](s *Service, data []byte) (T, error) {
	var dest T
	if err := s.Decode(data, &dest); err != nil {
		var zero T
		return zero, err
	}
	return dest, nil
}

// Encode serializes a value to JSON bytes.
func Encode(v any) ([]byte, error) {
	return defaultService.Encode(v)
}

func (s *Service) Encode(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encode json: %w", err)
	}
	return data, nil
}

func normalize(value any) any {
	switch v := value.(type) {
	case json.Number:
		return numberToValue(v)
	case map[string]any:
		for k, item := range v {
			v[k] = normalize(item)
		}
		return v
	case []any:
		for i, item := range v {
			v[i] = normalize(item)
		}
		return v
	default:
		return v
	}
}

func numberToValue(n json.Number) any {
	if i, err := n.Int64(); err == nil {
		return i
	}
	if f, err := n.Float64(); err == nil {
		return f
	}
	return n.String()
}

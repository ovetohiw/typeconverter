package schema

import (
	"fmt"

	"typeconverter/jsonconv"
)

type Template struct {
	Root  TemplateField           `json:"root"`
	Types map[string]TemplateType `json:"types"`
}

type TemplateType struct {
	Kind      string          `json:"kind"`
	Abstract  bool            `json:"abstract,omitempty"`
	Base      string          `json:"base,omitempty"`
	Primitive string          `json:"primitive,omitempty"`
	Enum      []string        `json:"enum,omitempty"`
	Attrs     []TemplateField `json:"attributes,omitempty"`
	Elems     []TemplateField `json:"elements,omitempty"`
}

type TemplateField struct {
	Name     string          `json:"name,omitempty"`
	Type     string          `json:"type,omitempty"`
	Min      *int            `json:"min,omitempty"`
	Max      any             `json:"max,omitempty"`
	Nillable bool            `json:"nillable,omitempty"`
	Required bool            `json:"required,omitempty"`
	Choice   []TemplateField `json:"choice,omitempty"`
}

func EncodeTemplate(s *Schema) ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("nil schema")
	}
	root, err := s.RootField()
	if err != nil {
		return nil, err
	}
	tpl := Template{
		Root:  toTplField(root),
		Types: make(map[string]TemplateType, len(s.Types)),
	}
	for name, t := range s.Types {
		tpl.Types[name] = toTplType(t)
	}
	return jsonconv.Encode(tpl)
}

func ParseTemplate(data []byte) (*Schema, error) {
	tpl, err := jsonconv.DecodeTo[Template](data)
	if err != nil {
		return nil, err
	}
	if tpl.Root.Name == "" {
		return nil, fmt.Errorf("jsontemplate root.name is empty")
	}
	s := &Schema{
		Root:  tpl.Root.Name,
		Elems: []Field{fromTplField(tpl.Root)},
		Types: make(map[string]*Type, len(tpl.Types)),
	}
	for name, t := range tpl.Types {
		s.Types[name] = fromTplType(name, t)
	}
	return s, nil
}

func toTplType(t *Type) TemplateType {
	out := TemplateType{
		Kind:      t.Kind,
		Abstract:  t.Abstract,
		Base:      t.Base,
		Primitive: t.Primitive,
		Enum:      t.Enum,
	}
	for _, a := range t.Attrs {
		out.Attrs = append(out.Attrs, toTplField(a))
	}
	for _, e := range t.Elems {
		out.Elems = append(out.Elems, toTplField(e))
	}
	return out
}

func fromTplType(name string, t TemplateType) *Type {
	out := &Type{
		Name:      name,
		Kind:      t.Kind,
		Abstract:  t.Abstract,
		Base:      t.Base,
		Primitive: t.Primitive,
		Enum:      t.Enum,
	}
	if out.Kind == "" {
		if len(out.Attrs) > 0 || len(t.Elems) > 0 || out.Base != "" {
			out.Kind = "complex"
		} else {
			out.Kind = "simple"
		}
	}
	for _, a := range t.Attrs {
		out.Attrs = append(out.Attrs, fromTplField(a))
	}
	for _, e := range t.Elems {
		out.Elems = append(out.Elems, fromTplField(e))
	}
	return out
}

func toTplField(f Field) TemplateField {
	out := TemplateField{
		Name:     f.Name,
		Type:     f.Type,
		Nillable: f.Nillable,
		Required: f.Required,
	}
	if f.Min != 1 {
		min := f.Min
		out.Min = &min
	}
	switch {
	case f.Max == Unbounded:
		out.Max = "unbounded"
	case f.Max != 1:
		out.Max = f.Max
	}
	for _, c := range f.Choice {
		out.Choice = append(out.Choice, toTplField(c))
	}
	return out
}

func fromTplField(f TemplateField) Field {
	out := Field{
		Name:     f.Name,
		Type:     f.Type,
		Min:      1,
		Max:      1,
		Nillable: f.Nillable,
		Required: f.Required,
	}
	if f.Min != nil {
		out.Min = *f.Min
	}
	out.Max = maxFromAny(f.Max)
	for _, c := range f.Choice {
		out.Choice = append(out.Choice, fromTplField(c))
	}
	if out.Type == "" && len(out.Choice) == 0 {
		out.Type = "string"
	}
	return out
}

func maxFromAny(v any) int {
	switch n := v.(type) {
	case nil:
		return 1
	case string:
		if n == "unbounded" {
			return Unbounded
		}
		return occurs(n, 1)
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 1
	}
}

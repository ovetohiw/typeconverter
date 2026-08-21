package schema

import "fmt"

const Unbounded = -1

type Schema struct {
	Root  string
	Elems []Field
	Types map[string]*Type
}

type Type struct {
	Name      string
	Kind      string
	Abstract  bool
	Base      string
	Primitive string
	Enum      []string
	Attrs     []Field
	Elems     []Field
}

type Field struct {
	Name     string
	Type     string
	Min      int
	Max      int
	Nillable bool
	Required bool
	Choice   []Field
}

func (t *Type) Scalar() bool {
	return t != nil && (t.Kind == "primitive" || t.Kind == "simple")
}

func (f Field) Repeated() bool {
	return f.Max == Unbounded || f.Max > 1
}

func (s *Schema) RootField() (Field, error) {
	if s == nil || len(s.Elems) == 0 {
		return Field{}, fmt.Errorf("schema has no root element")
	}
	for _, el := range s.Elems {
		if el.Name == s.Root || s.Root == "" {
			return el, nil
		}
	}
	return s.Elems[0], nil
}

func (s *Schema) Lookup(name string) (*Type, error) {
	name = localName(name)
	if name == "" {
		return nil, fmt.Errorf("empty type name")
	}
	if s != nil && s.Types != nil {
		if t, ok := s.Types[name]; ok {
			return t, nil
		}
	}
	if t, ok := builtinType(name); ok {
		return t, nil
	}
	return nil, fmt.Errorf("unknown type %q", name)
}

func (s *Schema) DerivedFrom(name, base string) bool {
	name, base = localName(name), localName(base)
	seen := map[string]bool{}
	for name != "" {
		if name == base {
			return true
		}
		if seen[name] {
			return false
		}
		seen[name] = true
		t, err := s.Lookup(name)
		if err != nil || t.Base == "" {
			return false
		}
		name = t.Base
	}
	return false
}

func (s *Schema) Effective(t *Type) (attrs, elems []Field, err error) {
	if t == nil {
		return nil, nil, fmt.Errorf("nil type")
	}
	var walk func(*Type, map[string]bool) error
	walk = func(cur *Type, seen map[string]bool) error {
		if cur.Base != "" {
			if seen[cur.Base] {
				return fmt.Errorf("type cycle at %s", cur.Name)
			}
			seen[cur.Base] = true
			base, err := s.Lookup(cur.Base)
			if err != nil {
				return err
			}
			if err := walk(base, seen); err != nil {
				return err
			}
		}
		attrs = append(attrs, cur.Attrs...)
		elems = append(elems, cur.Elems...)
		return nil
	}
	if err := walk(t, map[string]bool{}); err != nil {
		return nil, nil, err
	}
	return attrs, elems, nil
}

func builtinType(name string) (*Type, bool) {
	switch name {
	case "string", "int", "integer", "long", "short", "byte", "unsignedInt", "unsignedLong",
		"boolean", "decimal", "float", "double", "date", "dateTime", "time", "gYear",
		"duration", "anyURI", "base64Binary", "token", "normalizedString":
		return &Type{Name: name, Kind: "primitive", Primitive: name}, true
	default:
		return nil, false
	}
}

func localName(name string) string {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == ':' {
			return name[i+1:]
		}
	}
	return name
}

package schema

import (
	"fmt"
	"strings"

	"typeconverter/converter"
	"typeconverter/jsonconv"
	"typeconverter/xmlconv"
)

const (
	xsiNS      = "http://www.w3.org/2001/XMLSchema-instance"
	attrPrefix = "@"
	typeKey    = "$type"
)

type Instance struct {
	Schema *Schema
	Root   string
	Value  any
}

func Decode(s *Schema, format string, data []byte) (*Instance, error) {
	switch strings.ToLower(format) {
	case "xml":
		return s.DecodeXML(data)
	case "json":
		return s.DecodeJSON(data)
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
}

func (s *Schema) DecodeXML(data []byte) (*Instance, error) {
	root, err := xmlconv.Parse(data)
	if err != nil {
		return nil, err
	}
	field, err := s.RootField()
	if err != nil {
		return nil, err
	}
	if root.Name != field.Name {
		return nil, fmt.Errorf("xml root %q is not %q", root.Name, field.Name)
	}
	val, err := s.bindXMLNode(root, field)
	if err != nil {
		return nil, err
	}
	return &Instance{Schema: s, Root: field.Name, Value: val}, nil
}

func (s *Schema) DecodeJSON(data []byte) (*Instance, error) {
	raw, err := jsonconv.Parse(data)
	if err != nil {
		return nil, err
	}
	field, err := s.RootField()
	if err != nil {
		return nil, err
	}
	src := raw
	if m, ok := raw.(map[string]any); ok {
		if inner, ok := m[field.Name]; ok {
			src = inner
		}
	}
	val, err := s.bindJSONValue(src, field)
	if err != nil {
		return nil, err
	}
	return &Instance{Schema: s, Root: field.Name, Value: val}, nil
}

func (in *Instance) EncodeXML() ([]byte, error) {
	if in == nil || in.Schema == nil {
		return nil, fmt.Errorf("nil schema instance")
	}
	field, err := in.Schema.RootField()
	if err != nil {
		return nil, err
	}
	node, err := in.Schema.xmlNode(field.Name, in.Value, field)
	if err != nil {
		return nil, err
	}
	return xmlconv.Encode(node)
}

func (in *Instance) EncodeJSON() ([]byte, error) {
	if in == nil {
		return nil, fmt.Errorf("nil schema instance")
	}
	return jsonconv.Encode(map[string]any{in.Root: in.Value})
}

func (s *Schema) bindXMLNode(node *xmlconv.Node, field Field) (any, error) {
	if field.Nillable && isXSINil(node) {
		return nil, nil
	}
	t, err := s.declaredType(node, field)
	if err != nil {
		return nil, err
	}
	if t.Scalar() {
		return s.bindScalar(node.Content(), t)
	}
	return s.bindXMLComplex(node, t)
}

func (s *Schema) declaredType(node *xmlconv.Node, field Field) (*Type, error) {
	declared, err := s.Lookup(field.Type)
	if err != nil {
		return nil, fmt.Errorf("element %s: %w", field.Name, err)
	}
	xt, ok := xsiType(node)
	if !ok {
		if declared.Abstract {
			return nil, fmt.Errorf("element %s: abstract type %s requires xsi:type", field.Name, declared.Name)
		}
		return declared, nil
	}
	xt = localName(xt)
	actual, err := s.Lookup(xt)
	if err != nil {
		return nil, fmt.Errorf("element %s: %w", field.Name, err)
	}
	if !s.DerivedFrom(actual.Name, declared.Name) {
		return nil, fmt.Errorf("element %s: type %s is not derived from %s", field.Name, actual.Name, declared.Name)
	}
	return actual, nil
}

func (s *Schema) bindXMLComplex(node *xmlconv.Node, t *Type) (any, error) {
	attrs, elems, err := s.Effective(t)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if xt, ok := xsiType(node); ok {
		out[typeKey] = localName(xt)
	}
	for _, a := range attrs {
		raw, ok := node.Attr(a.Name)
		if !ok {
			continue
		}
		at, err := s.Lookup(a.Type)
		if err != nil {
			return nil, fmt.Errorf("attribute %s: %w", a.Name, err)
		}
		val, err := s.bindScalar(raw, at)
		if err != nil {
			return nil, fmt.Errorf("attribute %s: %w", a.Name, err)
		}
		out[attrPrefix+a.Name] = val
	}
	for _, e := range elems {
		if len(e.Choice) > 0 {
			val, err := s.bindXMLChoice(node, e)
			if err != nil {
				return nil, err
			}
			mergeChoice(out, val)
			continue
		}
		kids := node.ChildrenByName(e.Name)
		if len(kids) == 0 {
			continue
		}
		val, err := s.bindXMLRepeats(kids, e)
		if err != nil {
			return nil, fmt.Errorf("element %s: %w", e.Name, err)
		}
		out[e.Name] = val
	}
	return out, nil
}

func (s *Schema) bindXMLChoice(node *xmlconv.Node, field Field) (map[string]any, error) {
	for _, opt := range field.Choice {
		kids := node.ChildrenByName(opt.Name)
		if len(kids) == 0 {
			continue
		}
		val, err := s.bindXMLRepeats(kids, opt)
		if err != nil {
			return nil, fmt.Errorf("choice %s: %w", opt.Name, err)
		}
		return map[string]any{opt.Name: val}, nil
	}
	if field.Min == 0 {
		return nil, nil
	}
	return nil, fmt.Errorf("required choice is missing")
}

func (s *Schema) bindXMLRepeats(nodes []*xmlconv.Node, field Field) (any, error) {
	if !field.Repeated() {
		return s.bindXMLNode(nodes[0], field)
	}
	out := make([]any, 0, len(nodes))
	for _, n := range nodes {
		val, err := s.bindXMLNode(n, field)
		if err != nil {
			return nil, err
		}
		out = append(out, val)
	}
	return out, nil
}

func (s *Schema) bindJSONValue(src any, field Field) (any, error) {
	if src == nil {
		if field.Nillable || field.Min == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("element %s is required", field.Name)
	}
	if field.Repeated() {
		arr, ok := src.([]any)
		if !ok {
			arr = []any{src}
		}
		out := make([]any, 0, len(arr))
		item := field
		item.Max = 1
		for _, v := range arr {
			val, err := s.bindJSONValue(v, item)
			if err != nil {
				return nil, err
			}
			out = append(out, val)
		}
		return out, nil
	}
	t, err := s.Lookup(field.Type)
	if err != nil {
		return nil, fmt.Errorf("element %s: %w", field.Name, err)
	}
	if m, ok := src.(map[string]any); ok {
		if xt, ok := m[typeKey]; ok {
			name, _ := converter.Convert[string](xt)
			actual, err := s.Lookup(name)
			if err != nil {
				return nil, fmt.Errorf("element %s: %w", field.Name, err)
			}
			if !s.DerivedFrom(actual.Name, t.Name) {
				return nil, fmt.Errorf("element %s: type %s is not derived from %s", field.Name, actual.Name, t.Name)
			}
			t = actual
		}
	}
	if t.Scalar() {
		return s.bindScalar(src, t)
	}
	m, ok := src.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("element %s: expected object, got %T", field.Name, src)
	}
	return s.bindJSONComplex(m, t)
}

func (s *Schema) bindJSONComplex(src map[string]any, t *Type) (any, error) {
	attrs, elems, err := s.Effective(t)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if xt, ok := src[typeKey]; ok {
		out[typeKey] = xt
	}
	for _, a := range attrs {
		raw, ok := lookupJSON(src, attrPrefix+a.Name, a.Name)
		if !ok {
			continue
		}
		at, err := s.Lookup(a.Type)
		if err != nil {
			return nil, fmt.Errorf("attribute %s: %w", a.Name, err)
		}
		val, err := s.bindScalar(raw, at)
		if err != nil {
			return nil, fmt.Errorf("attribute %s: %w", a.Name, err)
		}
		out[attrPrefix+a.Name] = val
	}
	for _, e := range elems {
		if len(e.Choice) > 0 {
			val, err := s.bindJSONChoice(src, e)
			if err != nil {
				return nil, err
			}
			mergeChoice(out, val)
			continue
		}
		raw, ok := src[e.Name]
		if !ok {
			continue
		}
		val, err := s.bindJSONValue(raw, e)
		if err != nil {
			return nil, fmt.Errorf("element %s: %w", e.Name, err)
		}
		out[e.Name] = val
	}
	return out, nil
}

func (s *Schema) bindJSONChoice(src map[string]any, field Field) (map[string]any, error) {
	for _, opt := range field.Choice {
		raw, ok := src[opt.Name]
		if !ok {
			continue
		}
		val, err := s.bindJSONValue(raw, opt)
		if err != nil {
			return nil, fmt.Errorf("choice %s: %w", opt.Name, err)
		}
		return map[string]any{opt.Name: val}, nil
	}
	if field.Min == 0 {
		return nil, nil
	}
	return nil, fmt.Errorf("required choice is missing")
}

func (s *Schema) xmlNode(name string, val any, field Field) (*xmlconv.Node, error) {
	if val == nil {
		n := &xmlconv.Node{Name: name}
		if field.Nillable {
			setXSI(n, "nil", "true")
		}
		return n, nil
	}
	if field.Repeated() {
		return nil, fmt.Errorf("internal: repeated field %s passed to xmlNode", name)
	}
	t, err := s.Lookup(field.Type)
	if err != nil {
		return nil, err
	}
	if m, ok := val.(map[string]any); ok {
		if xt, ok := m[typeKey]; ok {
			nameType, _ := converter.Convert[string](xt)
			actual, err := s.Lookup(nameType)
			if err == nil && s.DerivedFrom(actual.Name, t.Name) {
				t = actual
			}
		}
	}
	n := &xmlconv.Node{Name: name}
	if t.Scalar() {
		n.Text = scalarString(val)
		return n, nil
	}
	m, ok := val.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("element %s: expected object, got %T", name, val)
	}
	if xt, ok := m[typeKey]; ok {
		setXSI(n, "type", scalarString(xt))
	}
	attrs, elems, err := s.Effective(t)
	if err != nil {
		return nil, err
	}
	for _, a := range attrs {
		raw, ok := lookupJSON(m, attrPrefix+a.Name, a.Name)
		if !ok {
			continue
		}
		n.Attrs = append(n.Attrs, xmlconv.Attr{Name: a.Name, Value: scalarString(raw)})
	}
	for _, e := range elems {
		if len(e.Choice) > 0 {
			kids, err := s.xmlChoice(m, e)
			if err != nil {
				return nil, err
			}
			n.Children = append(n.Children, kids...)
			continue
		}
		raw, ok := m[e.Name]
		if !ok {
			continue
		}
		kids, err := s.xmlRepeats(e.Name, raw, e)
		if err != nil {
			return nil, err
		}
		n.Children = append(n.Children, kids...)
	}
	return n, nil
}

func (s *Schema) xmlChoice(src map[string]any, field Field) ([]*xmlconv.Node, error) {
	for _, opt := range field.Choice {
		raw, ok := src[opt.Name]
		if !ok {
			continue
		}
		return s.xmlRepeats(opt.Name, raw, opt)
	}
	return nil, nil
}

func (s *Schema) xmlRepeats(name string, val any, field Field) ([]*xmlconv.Node, error) {
	if field.Repeated() {
		arr, ok := val.([]any)
		if !ok {
			arr = []any{val}
		}
		out := make([]*xmlconv.Node, 0, len(arr))
		item := field
		item.Max = 1
		for _, v := range arr {
			n, err := s.xmlNode(name, v, item)
			if err != nil {
				return nil, err
			}
			out = append(out, n)
		}
		return out, nil
	}
	n, err := s.xmlNode(name, val, field)
	if err != nil {
		return nil, err
	}
	return []*xmlconv.Node{n}, nil
}

func (s *Schema) bindScalar(v any, t *Type) (any, error) {
	prim := t.Primitive
	if t.Kind == "primitive" || prim == "" {
		prim = t.Name
	}
	switch prim {
	case "int", "integer", "long", "short", "byte", "unsignedInt", "unsignedLong":
		return converter.Convert[int64](v)
	case "decimal", "float", "double":
		return converter.Convert[float64](v)
	case "boolean":
		return converter.Convert[bool](v)
	default:
		return converter.Convert[string](v)
	}
}

func mergeChoice(out map[string]any, val map[string]any) {
	for k, v := range val {
		out[k] = v
	}
}

func lookupJSON(m map[string]any, names ...string) (any, bool) {
	for _, name := range names {
		if v, ok := m[name]; ok {
			return v, true
		}
	}
	return nil, false
}

func scalarString(v any) string {
	s, err := converter.Convert[string](v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return s
}

func xsiType(n *xmlconv.Node) (string, bool) {
	return xsiAttr(n, "type")
}

func isXSINil(n *xmlconv.Node) bool {
	v, ok := xsiAttr(n, "nil")
	return ok && isTrue(v)
}

func xsiAttr(n *xmlconv.Node, local string) (string, bool) {
	for _, a := range n.Attrs {
		if a.Name != local {
			continue
		}
		if a.Space == xsiNS || strings.Contains(strings.ToLower(a.Space), "xmlschema-instance") {
			return a.Value, true
		}
	}
	return "", false
}

func setXSI(n *xmlconv.Node, local, value string) {
	hasNS := false
	for _, a := range n.Attrs {
		if a.Name == "xmlns:xsi" {
			hasNS = true
			break
		}
	}
	if !hasNS {
		n.Attrs = append(n.Attrs, xmlconv.Attr{Name: "xmlns:xsi", Value: xsiNS})
	}
	n.Attrs = append(n.Attrs, xmlconv.Attr{Name: "xsi:" + local, Value: value})
}

package schema

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"typeconverter/xmlconv"
)

func ParseXSD(data []byte) (*Schema, error) {
	root, err := xmlconv.Parse(data)
	if err != nil {
		return nil, err
	}
	if root.Name != "schema" {
		return nil, fmt.Errorf("xml root %q is not schema", root.Name)
	}
	p := &xsdParser{schema: &Schema{Types: make(map[string]*Type)}}
	for _, n := range root.Children {
		switch n.Name {
		case "simpleType":
			name := xmlAttr(n, "name")
			if name == "" {
				return nil, fmt.Errorf("global simpleType without name")
			}
			p.schema.Types[name] = p.parseSimple(n, name)
		case "complexType":
			name := xmlAttr(n, "name")
			if name == "" {
				return nil, fmt.Errorf("global complexType without name")
			}
			p.schema.Types[name] = p.parseComplex(n, name, name)
		}
	}
	for _, n := range root.ChildrenByName("element") {
		p.schema.Elems = append(p.schema.Elems, p.parseElement(n, ""))
	}
	if len(p.schema.Elems) == 0 {
		return nil, fmt.Errorf("xsd has no root element")
	}
	p.schema.Root = p.schema.Elems[0].Name
	return p.schema, nil
}

type xsdParser struct {
	schema *Schema
}

func (p *xsdParser) parseSimple(n *xmlconv.Node, name string) *Type {
	t := &Type{Name: name, Kind: "simple", Primitive: "string"}
	if rest := n.Child("restriction"); rest != nil {
		t.Primitive = localName(xmlAttr(rest, "base"))
		if t.Primitive == "" {
			t.Primitive = "string"
		}
		for _, e := range rest.ChildrenByName("enumeration") {
			if v := xmlAttr(e, "value"); v != "" {
				t.Enum = append(t.Enum, v)
			}
		}
	}
	return t
}

func (p *xsdParser) parseComplex(n *xmlconv.Node, name, path string) *Type {
	t := &Type{Name: name, Kind: "complex", Abstract: isTrue(xmlAttr(n, "abstract"))}
	if cc := n.Child("complexContent"); cc != nil {
		if ext := cc.Child("extension"); ext != nil {
			t.Base = localName(xmlAttr(ext, "base"))
			p.fill(ext, t, path)
			return t
		}
		if rest := cc.Child("restriction"); rest != nil {
			t.Base = localName(xmlAttr(rest, "base"))
			p.fill(rest, t, path)
			return t
		}
	}
	p.fill(n, t, path)
	return t
}

func (p *xsdParser) fill(n *xmlconv.Node, t *Type, path string) {
	for _, a := range n.ChildrenByName("attribute") {
		if xmlAttr(a, "use") == "prohibited" {
			continue
		}
		t.Attrs = append(t.Attrs, Field{
			Name:     xmlAttr(a, "name"),
			Type:     typeRef(xmlAttr(a, "type")),
			Min:      0,
			Max:      1,
			Required: xmlAttr(a, "use") == "required",
		})
	}
	if seq := n.Child("sequence"); seq != nil {
		p.fillGroup(seq, t, path)
	}
	if ch := n.Child("choice"); ch != nil && n.Child("sequence") == nil {
		t.Elems = append(t.Elems, p.parseChoice(ch, path))
	}
}

func (p *xsdParser) fillGroup(n *xmlconv.Node, t *Type, path string) {
	for _, c := range n.Children {
		switch c.Name {
		case "element":
			t.Elems = append(t.Elems, p.parseElement(c, path))
		case "choice":
			t.Elems = append(t.Elems, p.parseChoice(c, path))
		case "sequence":
			p.fillGroup(c, t, path)
		}
	}
}

func (p *xsdParser) parseChoice(n *xmlconv.Node, path string) Field {
	f := Field{Min: occurs(xmlAttr(n, "minOccurs"), 1), Max: maxOccurs(xmlAttr(n, "maxOccurs"), 1)}
	for _, el := range n.ChildrenByName("element") {
		f.Choice = append(f.Choice, p.parseElement(el, path))
	}
	return f
}

func (p *xsdParser) parseElement(n *xmlconv.Node, path string) Field {
	name := xmlAttr(n, "name")
	f := Field{
		Name:     name,
		Type:     typeRef(xmlAttr(n, "type")),
		Min:      occurs(xmlAttr(n, "minOccurs"), 1),
		Max:      maxOccurs(xmlAttr(n, "maxOccurs"), 1),
		Nillable: isTrue(xmlAttr(n, "nillable")),
	}
	childPath := name
	if path != "" && name != "" {
		childPath = path + "." + name
	}
	if ct := n.Child("complexType"); ct != nil {
		t := p.parseComplex(ct, childPath, childPath)
		p.schema.Types[t.Name] = t
		f.Type = t.Name
	}
	if st := n.Child("simpleType"); st != nil {
		t := p.parseSimple(st, childPath)
		p.schema.Types[t.Name] = t
		f.Type = t.Name
	}
	if f.Type == "" {
		f.Type = "string"
	}
	return f
}

func xmlAttr(n *xmlconv.Node, name string) string {
	v, _ := n.Attr(name)
	return v
}

func typeRef(raw string) string {
	name := localName(raw)
	if name == "" {
		return ""
	}
	return name
}

func isTrue(v string) bool {
	return strings.EqualFold(v, "true") || v == "1"
}

func occurs(raw string, def int) int {
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return n
}

func maxOccurs(raw string, def int) int {
	if strings.EqualFold(raw, "unbounded") {
		return Unbounded
	}
	return occurs(raw, def)
}

func EncodeXSD(s *Schema) ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("nil schema")
	}
	root, err := s.RootField()
	if err != nil {
		return nil, err
	}
	n := &xmlconv.Node{
		Name: "schema",
		Attrs: []xmlconv.Attr{
			{Name: "elementFormDefault", Value: "qualified"},
			{Name: "xmlns:xs", Value: "http://www.w3.org/2001/XMLSchema"},
		},
	}
	n.Children = append(n.Children, xsdElement(root))
	names := make([]string, 0, len(s.Types))
	for name := range s.Types {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t := s.Types[name]
		if t.Kind == "simple" {
			n.Children = append(n.Children, xsdSimple(t))
			continue
		}
		if t.Kind == "complex" {
			n.Children = append(n.Children, xsdComplex(t))
		}
	}
	return xmlconv.Encode(n)
}

func xsdElement(f Field) *xmlconv.Node {
	n := &xmlconv.Node{Name: "element", Attrs: []xmlconv.Attr{{Name: "name", Value: f.Name}}}
	if f.Type != "" {
		n.Attrs = append(n.Attrs, xmlconv.Attr{Name: "type", Value: xsdType(f.Type)})
	}
	n.Attrs = append(n.Attrs, occursAttrs(f)...)
	if f.Nillable {
		n.Attrs = append(n.Attrs, xmlconv.Attr{Name: "nillable", Value: "true"})
	}
	return n
}

func xsdSimple(t *Type) *xmlconv.Node {
	n := &xmlconv.Node{Name: "simpleType", Attrs: []xmlconv.Attr{{Name: "name", Value: t.Name}}}
	rest := &xmlconv.Node{Name: "restriction", Attrs: []xmlconv.Attr{{Name: "base", Value: xsdType(t.Primitive)}}}
	for _, v := range t.Enum {
		rest.Children = append(rest.Children, &xmlconv.Node{Name: "enumeration", Attrs: []xmlconv.Attr{{Name: "value", Value: v}}})
	}
	n.Children = []*xmlconv.Node{rest}
	return n
}

func xsdComplex(t *Type) *xmlconv.Node {
	n := &xmlconv.Node{Name: "complexType", Attrs: []xmlconv.Attr{{Name: "name", Value: t.Name}}}
	if t.Abstract {
		n.Attrs = append(n.Attrs, xmlconv.Attr{Name: "abstract", Value: "true"})
	}
	body := n
	if t.Base != "" {
		cc := &xmlconv.Node{Name: "complexContent"}
		ext := &xmlconv.Node{Name: "extension", Attrs: []xmlconv.Attr{{Name: "base", Value: t.Base}}}
		cc.Children = []*xmlconv.Node{ext}
		n.Children = []*xmlconv.Node{cc}
		body = ext
	}
	if len(t.Elems) > 0 {
		seq := &xmlconv.Node{Name: "sequence"}
		for _, e := range t.Elems {
			if len(e.Choice) > 0 {
				ch := &xmlconv.Node{Name: "choice", Attrs: occursAttrs(e)}
				for _, opt := range e.Choice {
					ch.Children = append(ch.Children, xsdElement(opt))
				}
				seq.Children = append(seq.Children, ch)
				continue
			}
			seq.Children = append(seq.Children, xsdElement(e))
		}
		body.Children = append(body.Children, seq)
	}
	for _, a := range t.Attrs {
		an := &xmlconv.Node{Name: "attribute", Attrs: []xmlconv.Attr{
			{Name: "name", Value: a.Name},
			{Name: "type", Value: xsdType(a.Type)},
		}}
		if a.Required {
			an.Attrs = append(an.Attrs, xmlconv.Attr{Name: "use", Value: "required"})
		}
		body.Children = append(body.Children, an)
	}
	return n
}

func occursAttrs(f Field) []xmlconv.Attr {
	var out []xmlconv.Attr
	if f.Min != 1 {
		out = append(out, xmlconv.Attr{Name: "minOccurs", Value: strconv.Itoa(f.Min)})
	}
	if f.Max == Unbounded {
		out = append(out, xmlconv.Attr{Name: "maxOccurs", Value: "unbounded"})
	} else if f.Max != 1 {
		out = append(out, xmlconv.Attr{Name: "maxOccurs", Value: strconv.Itoa(f.Max)})
	}
	return out
}

func xsdType(name string) string {
	if _, ok := builtinType(name); ok {
		return "xs:" + name
	}
	return name
}

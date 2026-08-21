package xmlconv

import (
	"encoding/xml"
	"strings"
)

const (
	defaultAttrPrefix = "@"
	defaultTextKey    = "#text"
)

// Node is a generic XML element. It can represent any document shape:
// attributes, text, nested elements, and repeated children.
type Node struct {
	Name     string
	Space    string
	Attrs    []Attr
	Text     string
	Children []*Node
}

// Attr is an XML attribute.
type Attr struct {
	Name  string
	Space string
	Value string
}

func (n *Node) Attr(name string) (string, bool) {
	if n == nil {
		return "", false
	}
	for _, a := range n.Attrs {
		if a.Name == name {
			return a.Value, true
		}
	}
	return "", false
}

func (n *Node) Content() string {
	if n == nil {
		return ""
	}
	return strings.TrimSpace(n.Text)
}

func (n *Node) Child(name string) *Node {
	if n == nil {
		return nil
	}
	for _, c := range n.Children {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func (n *Node) ChildrenByName(name string) []*Node {
	if n == nil {
		return nil
	}
	var out []*Node
	for _, c := range n.Children {
		if c.Name == name {
			out = append(out, c)
		}
	}
	return out
}

func (n *Node) Find(path ...string) []*Node {
	if n == nil || len(path) == 0 {
		return nil
	}
	current := []*Node{n}
	for _, segment := range path {
		var next []*Node
		for _, node := range current {
			next = append(next, node.ChildrenByName(segment)...)
		}
		current = next
	}
	return current
}

func (n *Node) ToMap() map[string]any {
	return n.toMap(defaultAttrPrefix, defaultTextKey, true)
}

func (n *Node) toMap(attrPrefix, textKey string, trim bool) map[string]any {
	m := make(map[string]any)
	if n == nil {
		return m
	}
	for _, a := range n.Attrs {
		m[attrPrefix+a.Name] = a.Value
	}
	groups := groupChildren(n.Children)
	for name, nodes := range groups {
		if len(nodes) == 1 {
			m[name] = nodes[0].toValue(attrPrefix, textKey, trim)
			continue
		}
		items := make([]any, len(nodes))
		for i, child := range nodes {
			items[i] = child.toValue(attrPrefix, textKey, trim)
		}
		m[name] = items
	}
	text := n.Text
	if trim {
		text = strings.TrimSpace(text)
	}
	if text != "" {
		m[textKey] = text
	}
	return m
}

func (n *Node) toValue(attrPrefix, textKey string, trim bool) any {
	if n == nil {
		return nil
	}
	text := n.Text
	if trim {
		text = strings.TrimSpace(text)
	}
	if len(n.Attrs) == 0 && len(n.Children) == 0 {
		return text
	}
	return n.toMap(attrPrefix, textKey, trim)
}

func (n *Node) InnerXML() string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	writeMixed(&b, n)
	return b.String()
}

func writeMixed(b *strings.Builder, n *Node) {
	if n.Text != "" {
		_ = xml.EscapeText(b, []byte(n.Text))
	}
	for _, c := range n.Children {
		c.writeXML(b)
	}
}

func (n *Node) writeXML(b *strings.Builder) {
	b.WriteByte('<')
	b.WriteString(n.Name)
	for _, a := range n.Attrs {
		b.WriteByte(' ')
		b.WriteString(a.Name)
		b.WriteString(`="`)
		_ = xml.EscapeText(b, []byte(a.Value))
		b.WriteByte('"')
	}
	if len(n.Children) == 0 && n.Text == "" {
		b.WriteString("/>")
		return
	}
	b.WriteByte('>')
	writeMixed(b, n)
	b.WriteString("</")
	b.WriteString(n.Name)
	b.WriteByte('>')
}

func groupChildren(children []*Node) map[string][]*Node {
	groups := make(map[string][]*Node)
	for _, c := range children {
		groups[c.Name] = append(groups[c.Name], c)
	}
	return groups
}

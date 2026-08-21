package xmlconv

import (
	"encoding/xml"
	"fmt"
	"reflect"
	"strings"

	"typeconverter/converter"
)

func (s *Service) bind(root *Node, dest any) error {
	v := reflect.ValueOf(dest)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return fmt.Errorf("dest must be a non-nil pointer")
	}
	return s.bindValue(root, v.Elem(), "")
}

func (s *Service) bindValue(node *Node, dest reflect.Value, xmlName string) error {
	if !dest.IsValid() {
		return fmt.Errorf("invalid destination")
	}
	if dest.Kind() == reflect.Pointer {
		if dest.IsNil() {
			dest.Set(reflect.New(dest.Type().Elem()))
		}
		return s.bindValue(node, dest.Elem(), xmlName)
	}

	switch dest.Kind() {
	case reflect.Struct:
		if dest.Type() == reflect.TypeOf(xml.Name{}) {
			dest.Set(reflect.ValueOf(xml.Name{Space: node.Space, Local: node.Name}))
			return nil
		}
		if dest.Type() == reflect.TypeOf(Node{}) {
			dest.Set(reflect.ValueOf(*node))
			return nil
		}
		return s.bindStruct(node, dest)
	case reflect.Map:
		return s.bindMap(node, dest)
	case reflect.Slice, reflect.Array:
		return s.bindSlice(node, dest, xmlName)
	case reflect.Interface:
		if dest.NumMethod() != 0 {
			return fmt.Errorf("cannot bind XML into %s", dest.Type())
		}
		value := node.toValue(s.attrPrefix(), s.textKey(), s.trim())
		dest.Set(reflect.ValueOf(value))
		return nil
	default:
		return s.bindScalar(node.Content(), dest)
	}
}

func (s *Service) bindStruct(node *Node, dest reflect.Value) error {
	fields, err := collectFields(dest)
	if err != nil {
		return err
	}

	used := make(map[*Node]bool)
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
		if err := s.bindStructField(node, f, used); err != nil {
			return fmt.Errorf("field %s: %w", f.fieldName, err)
		}
	}

	if anyField != nil {
		if err := s.bindAnyField(node, anyField, used); err != nil {
			return fmt.Errorf("field %s: %w", anyField.fieldName, err)
		}
	}
	return nil
}

func (s *Service) bindStructField(node *Node, f *boundField, used map[*Node]bool) error {
	if f.value.Type() == reflect.TypeOf(xml.Name{}) {
		return s.bindValue(node, f.value, "")
	}
	switch {
	case f.attr:
		name := f.name
		if name == "" {
			name = f.fieldName
		}
		val, ok := node.Attr(name)
		if !ok {
			return nil
		}
		return s.bindScalar(val, f.value)
	case f.chardata:
		return s.bindScalar(node.Content(), f.value)
	case f.innerxml:
		return s.bindScalar(node.InnerXML(), f.value)
	}

	targets := s.selectNodes(node, f)
	for _, t := range targets {
		used[t] = true
	}

	if len(targets) == 0 {
		return nil
	}

	if f.value.Kind() == reflect.Slice || f.value.Kind() == reflect.Array {
		return s.setSlice(f.value, targets, f.leafName())
	}
	return s.bindValue(targets[0], f.value, f.leafName())
}

func (s *Service) bindAnyField(node *Node, f *boundField, used map[*Node]bool) error {
	var leftover []*Node
	for _, c := range node.Children {
		if !used[c] {
			leftover = append(leftover, c)
		}
	}
	if len(leftover) == 0 && len(node.Attrs) == 0 && strings.TrimSpace(node.Text) == "" {
		return nil
	}

	switch f.value.Kind() {
	case reflect.Slice:
		return s.setSlice(f.value, leftover, "")
	case reflect.Map, reflect.Interface, reflect.Struct, reflect.Pointer:
		virtual := &Node{
			Name:     node.Name,
			Space:    node.Space,
			Children: leftover,
		}
		return s.bindValue(virtual, f.value, "")
	default:
		if len(leftover) == 1 {
			return s.bindValue(leftover[0], f.value, leftover[0].Name)
		}
		return s.bindScalar(node.Content(), f.value)
	}
}

func (s *Service) selectNodes(node *Node, f *boundField) []*Node {
	path := f.path
	if len(path) == 0 {
		name := f.name
		if name == "" {
			name = f.fieldName
		}
		found := node.ChildrenByName(name)
		if len(found) == 0 {
			found = childrenByNameFold(node, name)
		}
		return found
	}
	return findPath(node, path)
}

func (s *Service) bindMap(node *Node, dest reflect.Value) error {
	if dest.IsNil() {
		dest.Set(reflect.MakeMap(dest.Type()))
	}
	if dest.Type().Key().Kind() != reflect.String {
		return fmt.Errorf("map key must be string, got %s", dest.Type().Key())
	}

	if dest.Type().Elem() == reflect.TypeOf((*any)(nil)).Elem() {
		for k, v := range node.toMap(s.attrPrefix(), s.textKey(), s.trim()) {
			dest.SetMapIndex(reflect.ValueOf(k), reflect.ValueOf(v))
		}
		return nil
	}

	elemType := dest.Type().Elem()
	for _, a := range node.Attrs {
		if err := s.setMapIndex(dest, s.attrPrefix()+a.Name, a.Value, elemType); err != nil {
			return err
		}
	}
	groups := groupChildren(node.Children)
	for name, nodes := range groups {
		if len(nodes) == 1 && elemType.Kind() != reflect.Slice {
			if err := s.setMapNode(dest, name, nodes[0], elemType); err != nil {
				return err
			}
			continue
		}
		if elemType.Kind() == reflect.Slice {
			slice := reflect.New(elemType).Elem()
			if err := s.setSlice(slice, nodes, name); err != nil {
				return err
			}
			dest.SetMapIndex(reflect.ValueOf(name), slice)
			continue
		}
		if elemType == reflect.TypeOf((*any)(nil)).Elem() {
			items := make([]any, len(nodes))
			for i, child := range nodes {
				items[i] = child.toValue(s.attrPrefix(), s.textKey(), s.trim())
			}
			dest.SetMapIndex(reflect.ValueOf(name), reflect.ValueOf(items))
			continue
		}
		if err := s.setMapNode(dest, name, nodes[0], elemType); err != nil {
			return err
		}
	}
	text := node.Content()
	if text != "" {
		return s.setMapIndex(dest, s.textKey(), text, elemType)
	}
	return nil
}

func (s *Service) setMapNode(dest reflect.Value, key string, node *Node, elemType reflect.Type) error {
	elem := reflect.New(elemType).Elem()
	if err := s.bindValue(node, elem, node.Name); err != nil {
		return err
	}
	dest.SetMapIndex(reflect.ValueOf(key), elem)
	return nil
}

func (s *Service) setMapIndex(dest reflect.Value, key, raw string, elemType reflect.Type) error {
	elem := reflect.New(elemType).Elem()
	if err := s.bindScalar(raw, elem); err != nil {
		return err
	}
	dest.SetMapIndex(reflect.ValueOf(key), elem)
	return nil
}

func (s *Service) bindSlice(node *Node, dest reflect.Value, xmlName string) error {
	children := node.Children
	if xmlName != "" {
		named := node.ChildrenByName(xmlName)
		if len(named) > 0 {
			children = named
		}
	}
	if len(children) == 0 && dest.Type().Elem().Kind() != reflect.Struct && dest.Type().Elem().Kind() != reflect.Map {
		if node.Content() == "" {
			return nil
		}
		return s.setSlice(dest, []*Node{node}, xmlName)
	}
	return s.setSlice(dest, children, xmlName)
}

func (s *Service) setSlice(dest reflect.Value, nodes []*Node, xmlName string) error {
	if dest.Kind() == reflect.Array {
		if len(nodes) > dest.Len() {
			return fmt.Errorf("array overflow: have %d values, array length %d", len(nodes), dest.Len())
		}
		for i, n := range nodes {
			if err := s.bindValue(n, dest.Index(i), xmlName); err != nil {
				return err
			}
		}
		return nil
	}

	slice := reflect.MakeSlice(dest.Type(), len(nodes), len(nodes))
	for i, n := range nodes {
		if err := s.bindValue(n, slice.Index(i), xmlName); err != nil {
			return err
		}
	}
	dest.Set(slice)
	return nil
}

func (s *Service) bindScalar(raw string, dest reflect.Value) error {
	if dest.Kind() == reflect.Pointer {
		if dest.IsNil() {
			dest.Set(reflect.New(dest.Type().Elem()))
		}
		return s.bindScalar(raw, dest.Elem())
	}
	if dest.Kind() == reflect.Interface && dest.NumMethod() == 0 {
		dest.Set(reflect.ValueOf(raw))
		return nil
	}

	switch dest.Kind() {
	case reflect.String:
		dest.SetString(raw)
		return nil
	case reflect.Bool:
		if raw == "" {
			dest.SetBool(false)
			return nil
		}
		v, err := converter.Convert[bool](raw)
		if err != nil {
			return err
		}
		dest.SetBool(v)
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if raw == "" {
			dest.SetInt(0)
			return nil
		}
		v, err := converter.Convert[int64](raw)
		if err != nil {
			return err
		}
		dest.SetInt(v)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if raw == "" {
			dest.SetUint(0)
			return nil
		}
		v, err := converter.Convert[int64](raw)
		if err != nil {
			return err
		}
		if v < 0 {
			return fmt.Errorf("cannot convert %q to %s", raw, dest.Type())
		}
		dest.SetUint(uint64(v))
		return nil
	case reflect.Float32, reflect.Float64:
		if raw == "" {
			dest.SetFloat(0)
			return nil
		}
		v, err := converter.Convert[float64](raw)
		if err != nil {
			return err
		}
		dest.SetFloat(v)
		return nil
	default:
		return fmt.Errorf("unsupported destination type %s", dest.Type())
	}
}

func (s *Service) attrPrefix() string {
	if s != nil && s.AttrPrefix != "" {
		return s.AttrPrefix
	}
	return defaultAttrPrefix
}

func (s *Service) textKey() string {
	if s != nil && s.TextKey != "" {
		return s.TextKey
	}
	return defaultTextKey
}

func (s *Service) trim() bool {
	if s == nil {
		return true
	}
	return s.TrimSpace
}

type boundField struct {
	value     reflect.Value
	index     []int
	fieldName string
	name      string
	path      []string
	attr      bool
	chardata  bool
	innerxml  bool
	any       bool
	ignore    bool
}

func (f *boundField) leafName() string {
	if len(f.path) > 0 {
		return f.path[len(f.path)-1]
	}
	if f.name != "" {
		return f.name
	}
	return f.fieldName
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
		if sf.Anonymous && sf.Type.Kind() == reflect.Struct && sf.Tag.Get("xml") == "" {
			nested, err := collectFields(fv)
			if err != nil {
				return nil, err
			}
			fields = append(fields, nested...)
			continue
		}
		bf, err := parseField(sf, fv, []int{i})
		if err != nil {
			return nil, err
		}
		fields = append(fields, bf)
	}
	return fields, nil
}

func parseField(sf reflect.StructField, fv reflect.Value, index []int) (boundField, error) {
	bf := boundField{
		value:     fv,
		index:     index,
		fieldName: sf.Name,
	}
	tag := sf.Tag.Get("xml")
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
		if strings.Contains(name, ">") {
			bf.path = strings.Split(name, ">")
		} else {
			bf.name = name
		}
	}
	for _, flag := range parts[1:] {
		switch strings.TrimSpace(flag) {
		case "attr":
			bf.attr = true
		case "chardata":
			bf.chardata = true
		case "innerxml":
			bf.innerxml = true
		case "any":
			bf.any = true
		case "omitempty", "comment":
		default:
			return bf, fmt.Errorf("unknown xml flag %q on field %s", flag, sf.Name)
		}
	}
	return bf, nil
}

func findPath(node *Node, path []string) []*Node {
	current := []*Node{node}
	for _, segment := range path {
		var next []*Node
		for _, n := range current {
			found := n.ChildrenByName(segment)
			if len(found) == 0 {
				found = childrenByNameFold(n, segment)
			}
			next = append(next, found...)
		}
		current = next
	}
	return current
}

func childrenByNameFold(node *Node, name string) []*Node {
	var out []*Node
	for _, c := range node.Children {
		if strings.EqualFold(c.Name, name) {
			out = append(out, c)
		}
	}
	return out
}

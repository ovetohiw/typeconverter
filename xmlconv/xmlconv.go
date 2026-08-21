package xmlconv

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"
)

// Service converts XML of any shape into DTO values: structs, slices, maps, or primitives.
type Service struct {
	TrimSpace  bool
	AttrPrefix string
	TextKey    string
}

func NewService() *Service {
	return &Service{
		TrimSpace:  true,
		AttrPrefix: defaultAttrPrefix,
		TextKey:    defaultTextKey,
	}
}

var defaultService = NewService()

// Parse reads XML into a generic node tree that can represent any document structure.
func Parse(data []byte) (*Node, error) {
	return defaultService.Parse(data)
}

func (s *Service) Parse(data []byte) (*Node, error) {
	return parseXML(bytes.NewReader(data))
}

// Decode unmarshals XML into dest. dest must be a non-nil pointer to a struct,
// map, slice, or primitive. Unknown nested fragments can be captured as
// map[string]any, any, or *Node fields.
func Decode(data []byte, dest any) error {
	return defaultService.Decode(data, dest)
}

func (s *Service) Decode(data []byte, dest any) error {
	root, err := s.Parse(data)
	if err != nil {
		return err
	}
	return s.bind(root, dest)
}

// DecodeFile reads an XML file and decodes it into dest.
func DecodeFile(path string, dest any) error {
	return defaultService.DecodeFile(path, dest)
}

func (s *Service) DecodeFile(path string, dest any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read xml file: %w", err)
	}
	return s.Decode(data, dest)
}

// DecodeTo unmarshals XML into a value of type T.
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

// Encode serializes a node tree to XML bytes.
func Encode(n *Node) ([]byte, error) {
	return defaultService.Encode(n)
}

func (s *Service) Encode(n *Node) ([]byte, error) {
	if n == nil {
		return nil, fmt.Errorf("encode xml: nil node")
	}
	var b strings.Builder
	b.WriteString(xml.Header)
	n.writeXML(&b)
	return []byte(b.String()), nil
}

func parseXML(r io.Reader) (*Node, error) {
	dec := xml.NewDecoder(r)
	var root *Node
	stack := make([]*Node, 0, 8)

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse xml: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			node := &Node{
				Name:  t.Name.Local,
				Space: t.Name.Space,
			}
			if len(t.Attr) > 0 {
				node.Attrs = make([]Attr, len(t.Attr))
				for i, a := range t.Attr {
					node.Attrs[i] = Attr{
						Name:  a.Name.Local,
						Space: a.Name.Space,
						Value: a.Value,
					}
				}
			}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, node)
			} else {
				root = node
			}
			stack = append(stack, node)
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, fmt.Errorf("parse xml: unexpected end element %s", t.Name.Local)
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) == 0 {
				continue
			}
			stack[len(stack)-1].Text += string(t)
		}
	}

	if root == nil {
		return nil, fmt.Errorf("parse xml: no root element")
	}
	return root, nil
}

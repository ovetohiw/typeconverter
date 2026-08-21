package model

import (
	"encoding/xml"
	"fmt"

	"typeconverter/jsonconv"
	"typeconverter/xmlconv"
)

// Catalog is the canonical document for both XML and JSON.
type Catalog struct {
	XMLName   xml.Name `xml:"catalog" json:"-"`
	Generated string   `xml:"generated,attr" json:"generated"`
	Version   int      `xml:"version,attr" json:"version"`
	Active    bool     `xml:"active,attr" json:"active"`
	Title     string   `xml:"title" json:"title"`
	Count     int      `xml:"count" json:"count"`
	Rating    float64  `xml:"rating" json:"rating"`
	Notes     string   `xml:"notes" json:"notes"`
	Flags     []string `xml:"flags>flag" json:"flags"`
	Books     []Book   `xml:"book" json:"books"`
}

type Book struct {
	ID        int       `xml:"id,attr" json:"id"`
	Available *bool     `xml:"available,attr" json:"available,omitempty"`
	SKU       string    `xml:"sku,attr" json:"sku,omitempty"`
	Title     string    `xml:"title" json:"title"`
	Subtitle  *string   `xml:"subtitle" json:"subtitle,omitempty"`
	Price     float64   `xml:"price,omitempty" json:"price,omitempty"`
	Pages     int       `xml:"pages,omitempty" json:"pages,omitempty"`
	Tags      []string  `xml:"tags>tag" json:"tags,omitempty"`
	Authors   []Author  `xml:"authors>author" json:"authors,omitempty"`
	Stock     []Stock   `xml:"stock" json:"stock,omitempty"`
	Editions  []Edition `xml:"edition" json:"editions,omitempty"`
	Meta      *Meta     `xml:"meta" json:"meta,omitempty"`
	Payload   *Payload  `xml:"payload" json:"payload,omitempty"`
}

type Author struct {
	ID     int    `xml:"id,attr" json:"id"`
	Name   string `xml:"name" json:"name"`
	Active bool   `xml:"active" json:"active"`
}

// Stock maps XML attribute+chardata onto a JSON object.
type Stock struct {
	SKU string `xml:"sku,attr" json:"sku"`
	Qty int    `xml:",chardata" json:"qty"`
}

// Edition is an attribute-only XML element and a JSON object.
type Edition struct {
	Year  int `xml:"year,attr" json:"year"`
	Pages int `xml:"pages,attr" json:"pages"`
}

type Meta struct {
	Author  string `xml:"author" json:"author"`
	Year    int    `xml:"year,omitempty" json:"year,omitempty"`
	Reprint *bool  `xml:"reprint" json:"reprint,omitempty"`
	Extra   *Extra `xml:"extra" json:"extra,omitempty"`
}

type Extra struct {
	ISBN string `xml:"isbn" json:"isbn"`
}

type Payload struct {
	Custom string `xml:"custom" json:"custom"`
	Nested Nested `xml:"nested" json:"nested"`
}

type Nested struct {
	Deep string `xml:"deep" json:"deep"`
}

func DecodeXML(data []byte) (Catalog, error) {
	cat, err := xmlconv.DecodeTo[Catalog](data)
	if err != nil {
		return Catalog{}, err
	}
	if cat.XMLName.Local != "" && cat.XMLName.Local != "catalog" {
		return Catalog{}, fmt.Errorf("xml root %q is not catalog", cat.XMLName.Local)
	}
	return cat, nil
}

func DecodeJSON(data []byte) (Catalog, error) {
	return jsonconv.DecodeTo[Catalog](data)
}

func (c Catalog) EncodeXML() ([]byte, error) {
	if c.XMLName.Local == "" {
		c.XMLName = xml.Name{Local: "catalog"}
	}
	body, err := xml.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("encode xml: %w", err)
	}
	out := make([]byte, 0, len(xml.Header)+len(body))
	out = append(out, xml.Header...)
	out = append(out, body...)
	return out, nil
}

func (c Catalog) EncodeJSON() ([]byte, error) {
	return jsonconv.Encode(c)
}

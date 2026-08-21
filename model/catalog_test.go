package model

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDecodeXMLAndJSONToSameCatalog(t *testing.T) {
	xmlSrc, err := os.ReadFile(filepath.Join("..", "xmlconv", "testdata", "catalog.xml"))
	if err != nil {
		t.Fatal(err)
	}
	jsonSrc, err := os.ReadFile(filepath.Join("..", "jsonconv", "testdata", "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}

	fromXML, err := DecodeXML(xmlSrc)
	if err != nil {
		t.Fatalf("DecodeXML: %v", err)
	}
	fromJSON, err := DecodeJSON(jsonSrc)
	if err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}

	fromXML.XMLName = fromJSON.XMLName
	if !reflect.DeepEqual(fromXML, fromJSON) {
		t.Fatalf("xml and json decoded differently\nxml: %#v\njson: %#v", fromXML, fromJSON)
	}
	assertCatalog(t, fromXML)
}

func TestEncodeRoundTripBothFormats(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "jsonconv", "testdata", "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := DecodeJSON(src)
	if err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}

	asXML, err := want.EncodeXML()
	if err != nil {
		t.Fatalf("EncodeXML: %v", err)
	}
	gotXML, err := DecodeXML(asXML)
	if err != nil {
		t.Fatalf("DecodeXML: %v", err)
	}
	gotXML.XMLName = want.XMLName
	if !reflect.DeepEqual(gotXML, want) {
		t.Fatalf("xml round-trip\n got %#v\nwant %#v", gotXML, want)
	}

	asJSON, err := want.EncodeJSON()
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	gotJSON, err := DecodeJSON(asJSON)
	if err != nil {
		t.Fatalf("DecodeJSON encoded: %v", err)
	}
	if !reflect.DeepEqual(gotJSON, want) {
		t.Fatalf("json round-trip\n got %#v\nwant %#v", gotJSON, want)
	}
}

func TestDecodeXMLRejectsOtherRoot(t *testing.T) {
	_, err := DecodeXML([]byte(`<order id="1"><title>x</title></order>`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func assertCatalog(t *testing.T, cat Catalog) {
	t.Helper()
	if cat.Generated != "2026-01-01" || cat.Version != 2 || !cat.Active {
		t.Fatalf("catalog attrs: %+v", cat)
	}
	if cat.Title != "Type Converter Catalog" || cat.Count != 3 || cat.Rating != 4.75 {
		t.Fatalf("catalog scalars: %+v", cat)
	}
	if cat.Notes != `Special chars: <tag> & "quotes"` {
		t.Fatalf("notes: %q", cat.Notes)
	}
	if !reflect.DeepEqual(cat.Flags, []string{"featured", "sale"}) {
		t.Fatalf("flags: %#v", cat.Flags)
	}
	if len(cat.Books) != 3 {
		t.Fatalf("books: %d", len(cat.Books))
	}

	first := cat.Books[0]
	if first.ID != 1 || first.SKU != "GO-001" || first.Title != "Go in Action" || first.Price != 32.5 || first.Pages != 300 {
		t.Fatalf("first book: %+v", first)
	}
	if first.Available == nil || !*first.Available {
		t.Fatal("expected available=true")
	}
	if !reflect.DeepEqual(first.Tags, []string{"lang", "backend"}) {
		t.Fatalf("tags: %#v", first.Tags)
	}
	if len(first.Authors) != 2 || first.Authors[0].ID != 10 || !first.Authors[0].Active || first.Authors[1].Active {
		t.Fatalf("authors: %+v", first.Authors)
	}
	if len(first.Stock) != 2 || first.Stock[0].SKU != "A" || first.Stock[0].Qty != 10 || first.Stock[1].Qty != 0 {
		t.Fatalf("stock: %+v", first.Stock)
	}

	second := cat.Books[1]
	if second.Title != "XML Cookbook" || second.Available != nil || second.Meta == nil {
		t.Fatalf("second book: %+v", second)
	}
	if second.Meta.Author != "Ann" || second.Meta.Year != 2020 {
		t.Fatalf("meta: %+v", second.Meta)
	}
	if second.Meta.Reprint == nil || *second.Meta.Reprint {
		t.Fatal("expected reprint=false")
	}
	if second.Meta.Extra == nil || second.Meta.Extra.ISBN != "123" {
		t.Fatalf("extra: %+v", second.Meta.Extra)
	}

	third := cat.Books[2]
	if third.Title != "Надёжный JSON" || third.Subtitle != nil {
		t.Fatalf("third book: %+v", third)
	}
	if third.Available == nil || *third.Available {
		t.Fatal("expected available=false")
	}
	if len(third.Editions) != 2 || third.Editions[0].Year != 2013 || third.Editions[1].Pages != 128 {
		t.Fatalf("editions: %+v", third.Editions)
	}
	if third.Payload == nil || third.Payload.Custom != "value" || third.Payload.Nested.Deep != "ok" {
		t.Fatalf("payload: %+v", third.Payload)
	}
}

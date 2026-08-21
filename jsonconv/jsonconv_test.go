package jsonconv

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDecodeStruct(t *testing.T) {
	data := []byte(`{
		"id": "42",
		"paid": true,
		"items": [
			{"sku": "A", "qty": 10},
			{"sku": "B", "qty": "20"}
		],
		"note": "urgent"
	}`)

	type Item struct {
		SKU string `json:"sku"`
		Qty int    `json:"qty"`
	}
	type Order struct {
		ID    int    `json:"id"`
		Paid  bool   `json:"paid"`
		Items []Item `json:"items"`
		Note  string `json:"note"`
	}

	got, err := DecodeTo[Order](data)
	if err != nil {
		t.Fatalf("DecodeTo: %v", err)
	}
	if got.ID != 42 || !got.Paid || got.Note != "urgent" {
		t.Fatalf("unexpected order: %+v", got)
	}
	if len(got.Items) != 2 || got.Items[0].SKU != "A" || got.Items[1].Qty != 20 {
		t.Fatalf("unexpected items: %+v", got.Items)
	}
}

func TestDecodeAnyStructureToMap(t *testing.T) {
	data := []byte(`{
		"user": {"id": 7, "name": "Ada"},
		"stats": {
			"clicks": 3,
			"nested": {"deep": "ok"}
		}
	}`)

	got, err := DecodeTo[map[string]any](data)
	if err != nil {
		t.Fatalf("DecodeTo: %v", err)
	}

	user, ok := got["user"].(map[string]any)
	if !ok {
		t.Fatalf("user: %T", got["user"])
	}
	if user["id"] != int64(7) || user["name"] != "Ada" {
		t.Fatalf("unexpected user: %#v", user)
	}

	stats, ok := got["stats"].(map[string]any)
	if !ok {
		t.Fatalf("stats: %T", got["stats"])
	}
	nested, ok := stats["nested"].(map[string]any)
	if !ok || nested["deep"] != "ok" {
		t.Fatalf("unexpected nested: %#v", stats)
	}
}

func TestDecodeMixedBooksFromFile(t *testing.T) {
	path := filepath.Join("testdata", "catalog.json")

	type Book struct {
		ID        int            `json:"id"`
		Available *bool          `json:"available"`
		Title     string         `json:"title"`
		Price     float64        `json:"price"`
		Tags      []string       `json:"tags"`
		Meta      map[string]any `json:"meta"`
	}
	type Catalog struct {
		Generated string `json:"generated"`
		Books     []Book `json:"books"`
	}

	var got Catalog
	if err := DecodeFile(path, &got); err != nil {
		t.Fatalf("DecodeFile: %v", err)
	}
	if got.Generated != "2026-01-01" || len(got.Books) != 3 {
		t.Fatalf("unexpected catalog: %+v", got)
	}
	if got.Books[0].Title != "Go in Action" || got.Books[0].Price != 32.5 {
		t.Fatalf("unexpected first book: %+v", got.Books[0])
	}
	if got.Books[0].Available == nil || !*got.Books[0].Available {
		t.Fatal("expected available=true on first book")
	}
	if !reflect.DeepEqual(got.Books[0].Tags, []string{"lang", "backend"}) {
		t.Fatalf("tags: %#v", got.Books[0].Tags)
	}
	if got.Books[1].Meta["author"] != "Ann" {
		t.Fatalf("meta: %#v", got.Books[1].Meta)
	}
	extra, ok := got.Books[1].Meta["extra"].(map[string]any)
	if !ok || extra["isbn"] != "123" {
		t.Fatalf("nested meta: %#v", got.Books[1].Meta)
	}
}

func TestDecodeAnyFieldAndPath(t *testing.T) {
	data := []byte(`{
		"kind": "event",
		"known": "yes",
		"payload": {"a": 1, "b": {"c": 2}},
		"user": {"profile": {"name": "Ada"}}
	}`)

	type Envelope struct {
		Kind    string         `json:"kind"`
		Known   string         `json:"known"`
		Name    string         `json:"user.profile.name"`
		Unknown map[string]any `json:",any"`
	}

	got, err := DecodeTo[Envelope](data)
	if err != nil {
		t.Fatalf("DecodeTo: %v", err)
	}
	if got.Kind != "event" || got.Known != "yes" || got.Name != "Ada" {
		t.Fatalf("unexpected envelope: %+v", got)
	}
	payload, ok := got.Unknown["payload"].(map[string]any)
	if !ok {
		t.Fatalf("payload: %#v", got.Unknown)
	}
	b, ok := payload["b"].(map[string]any)
	if !ok || b["c"] != int64(2) {
		t.Fatalf("nested payload: %#v", payload)
	}
}

func TestDecodeSliceAndPointers(t *testing.T) {
	got, err := DecodeTo[[]int]([]byte(`[1, "2", 3]`))
	if err != nil {
		t.Fatalf("DecodeTo: %v", err)
	}
	if !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Fatalf("got %#v", got)
	}

	type Holder struct {
		Value *int `json:"item"`
	}
	holder, err := DecodeTo[Holder]([]byte(`{"item": 9}`))
	if err != nil {
		t.Fatalf("pointer decode: %v", err)
	}
	if holder.Value == nil || *holder.Value != 9 {
		t.Fatalf("pointer: %+v", holder)
	}
}

func TestDecodeRawMessageAndNull(t *testing.T) {
	type Doc struct {
		Body json.RawMessage `json:"body"`
		Skip *string         `json:"skip"`
	}
	got, err := DecodeTo[Doc]([]byte(`{"body": {"x": 1}, "skip": null}`))
	if err != nil {
		t.Fatalf("DecodeTo: %v", err)
	}
	if string(got.Body) != `{"x":1}` {
		t.Fatalf("raw body: %s", got.Body)
	}
	if got.Skip != nil {
		t.Fatalf("expected nil skip, got %v", got.Skip)
	}
}

func TestParseTree(t *testing.T) {
	root, err := Parse([]byte(`{"a": [1, 2], "b": true}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	obj, ok := root.(map[string]any)
	if !ok {
		t.Fatalf("root: %T", root)
	}
	arr, ok := obj["a"].([]any)
	if !ok || len(arr) != 2 || arr[0] != int64(1) {
		t.Fatalf("array: %#v", obj["a"])
	}
	if obj["b"] != true {
		t.Fatalf("bool: %#v", obj["b"])
	}
}

func TestEncodeRoundTrip(t *testing.T) {
	src := []byte(`{"id": 42, "paid": true}`)
	value, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	encoded, err := Encode(value)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := DecodeTo[map[string]any](encoded)
	if err != nil {
		t.Fatalf("DecodeTo: %v", err)
	}
	if got["id"] != int64(42) || got["paid"] != true {
		t.Fatalf("got %#v", got)
	}
}

func TestDecodeInvalidJSON(t *testing.T) {
	_, err := DecodeTo[map[string]any]([]byte(`{`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDecodeRequiresPointer(t *testing.T) {
	var dest map[string]any
	if err := Decode([]byte(`{}`), dest); err == nil {
		t.Fatal("expected error")
	}
}

func TestDecodeFileMissing(t *testing.T) {
	var dest map[string]any
	err := DecodeFile(filepath.Join("testdata", "missing.json"), &dest)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDecodeCaseInsensitive(t *testing.T) {
	type User struct {
		Name string `json:"name"`
	}
	got, err := DecodeTo[User]([]byte(`{"Name": "Ada"}`))
	if err != nil {
		t.Fatalf("DecodeTo: %v", err)
	}
	if got.Name != "Ada" {
		t.Fatalf("got %+v", got)
	}
}

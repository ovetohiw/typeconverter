package xmlconv

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDecodeStruct(t *testing.T) {
	data := []byte(`
		<order id="42" paid="true">
			<item sku="A">10</item>
			<item sku="B">20</item>
			<note>urgent</note>
		</order>
	`)

	type Item struct {
		SKU string `xml:"sku,attr"`
		Qty int    `xml:",chardata"`
	}
	type Order struct {
		ID    int    `xml:"id,attr"`
		Paid  bool   `xml:"paid,attr"`
		Items []Item `xml:"item"`
		Note  string `xml:"note"`
	}

	got, err := DecodeTo[Order](data)
	if err != nil {
		t.Fatalf("DecodeTo: %v", err)
	}
	if got.ID != 42 || !got.Paid || got.Note != "urgent" {
		t.Fatalf("unexpected order: %+v", got)
	}
	if len(got.Items) != 2 || got.Items[0].SKU != "A" || got.Items[0].Qty != 10 {
		t.Fatalf("unexpected items: %+v", got.Items)
	}
}

func TestDecodeAnyStructureToMap(t *testing.T) {
	data := []byte(`
		<root>
			<user id="7">
				<name>Ada</name>
			</user>
			<stats>
				<clicks>3</clicks>
				<nested>
					<deep>ok</deep>
				</nested>
			</stats>
		</root>
	`)

	got, err := DecodeTo[map[string]any](data)
	if err != nil {
		t.Fatalf("DecodeTo: %v", err)
	}

	user, ok := got["user"].(map[string]any)
	if !ok {
		t.Fatalf("user: %T", got["user"])
	}
	if user["@id"] != "7" || user["name"] != "Ada" {
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
	path := filepath.Join("testdata", "catalog.xml")

	type Book struct {
		ID        int            `xml:"id,attr"`
		Available *bool          `xml:"available,attr"`
		Title     string         `xml:"title"`
		Price     float64        `xml:"price"`
		Tags      []string       `xml:"tags>tag"`
		Meta      map[string]any `xml:"meta"`
	}
	type Catalog struct {
		Generated string `xml:"generated,attr"`
		Books     []Book `xml:"book"`
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

func TestDecodeAnyField(t *testing.T) {
	data := []byte(`
		<envelope kind="event">
			<known>yes</known>
			<payload>
				<a>1</a>
				<b>
					<c>2</c>
				</b>
			</payload>
		</envelope>
	`)

	type Envelope struct {
		Kind    string         `xml:"kind,attr"`
		Known   string         `xml:"known"`
		Unknown map[string]any `xml:",any"`
	}

	got, err := DecodeTo[Envelope](data)
	if err != nil {
		t.Fatalf("DecodeTo: %v", err)
	}
	if got.Kind != "event" || got.Known != "yes" {
		t.Fatalf("unexpected envelope: %+v", got)
	}
	payload, ok := got.Unknown["payload"].(map[string]any)
	if !ok {
		t.Fatalf("payload: %#v", got.Unknown)
	}
	b, ok := payload["b"].(map[string]any)
	if !ok || b["c"] != "2" {
		t.Fatalf("nested payload: %#v", payload)
	}
}

func TestDecodeSliceAndPointers(t *testing.T) {
	data := []byte(`
		<list>
			<item>1</item>
			<item>2</item>
			<item>3</item>
		</list>
	`)

	got, err := DecodeTo[[]int](data)
	if err != nil {
		t.Fatalf("DecodeTo: %v", err)
	}
	if !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Fatalf("got %#v", got)
	}

	type Holder struct {
		Value *int `xml:"item"`
	}
	holder, err := DecodeTo[Holder]([]byte(`<root><item>9</item></root>`))
	if err != nil {
		t.Fatalf("pointer decode: %v", err)
	}
	if holder.Value == nil || *holder.Value != 9 {
		t.Fatalf("pointer: %+v", holder)
	}
}

func TestParseTree(t *testing.T) {
	root, err := Parse([]byte(`<a x="1"><b>hello</b><b>world</b></a>`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if root.Name != "a" {
		t.Fatalf("root name %q", root.Name)
	}
	val, ok := root.Attr("x")
	if !ok || val != "1" {
		t.Fatalf("attr: %q %v", val, ok)
	}
	bs := root.ChildrenByName("b")
	if len(bs) != 2 || bs[0].Content() != "hello" {
		t.Fatalf("children: %+v", bs)
	}
}

func TestEncodeRoundTrip(t *testing.T) {
	src := []byte(`<order id="42"><note>urgent</note></order>`)
	node, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	encoded, err := Encode(node)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Parse(encoded)
	if err != nil {
		t.Fatalf("Parse encoded: %v", err)
	}
	if got.Name != "order" {
		t.Fatalf("name %q", got.Name)
	}
	id, ok := got.Attr("id")
	if !ok || id != "42" {
		t.Fatalf("id: %q %v", id, ok)
	}
	if got.Child("note").Content() != "urgent" {
		t.Fatalf("encoded: %s", encoded)
	}
}

func TestEncodeNilNode(t *testing.T) {
	if _, err := Encode(nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestDecodeInvalidXML(t *testing.T) {
	_, err := DecodeTo[map[string]any]([]byte(`<a></b>`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDecodeRequiresPointer(t *testing.T) {
	var dest map[string]any
	if err := Decode([]byte(`<a/>`), dest); err == nil {
		t.Fatal("expected error")
	}
}

func TestDecodeFileMissing(t *testing.T) {
	var dest map[string]any
	err := DecodeFile(filepath.Join("testdata", "missing.xml"), &dest)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestServiceCustomKeys(t *testing.T) {
	svc := NewService()
	svc.AttrPrefix = "_"
	svc.TextKey = "text"

	var dest map[string]any
	if err := svc.Decode([]byte(`<n id="1">hello</n>`), &dest); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if dest["_id"] != "1" || dest["text"] != "hello" {
		t.Fatalf("got %#v", dest)
	}
}

func TestMainTestdataExists(t *testing.T) {
	path := filepath.Join("testdata", "catalog.xml")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

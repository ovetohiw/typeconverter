package schema

import (
	"os"
	"path/filepath"
	"testing"
)

func loadMessages(t *testing.T) *Schema {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "messages.xsd"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := ParseXSD(data)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestParseMessagesXSD(t *testing.T) {
	s := loadMessages(t)
	if s.Root != "Messages" {
		t.Fatalf("root %q", s.Root)
	}

	base, err := s.Lookup("PublisherInfoBase")
	if err != nil || !base.Abstract || len(base.Attrs) != 3 {
		t.Fatalf("PublisherInfoBase: %+v err=%v", base, err)
	}

	am, err := s.Lookup("Messages.MessageData.PublisherInfo.ArbitrManager")
	if err != nil {
		t.Fatal(err)
	}
	if am.Base != "PublisherInfoBase" {
		t.Fatalf("arbitr base %q", am.Base)
	}
	attrs, elems, err := s.Effective(am)
	if err != nil {
		t.Fatal(err)
	}
	if !hasField(attrs, "Id") || !hasField(attrs, "INN") || !hasField(elems, "OGRN") {
		t.Fatalf("effective arbitr attrs=%+v elems=%+v", attrs, elems)
	}

	pub, err := s.Lookup("Publisher.ArbitrManager.v2")
	if err != nil {
		t.Fatal(err)
	}
	if !s.DerivedFrom(pub.Name, "Publisher") {
		t.Fatal("expected Publisher.ArbitrManager.v2 derived from Publisher")
	}

	info, err := s.Lookup("Messages.MessageData.PublisherInfo")
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Elems) != 1 || len(info.Elems[0].Choice) != 2 {
		t.Fatalf("publisher choice: %+v", info.Elems)
	}
}

func TestParseEFRSBXSD(t *testing.T) {
	data, err := os.ReadFile(`c:\Users\red_b\Desktop\333.xsd`)
	if err != nil {
		t.Skip("333.xsd is not available")
	}
	s, err := ParseXSD(data)
	if err != nil {
		t.Fatal(err)
	}
	if s.Root != "Messages" {
		t.Fatalf("root %q", s.Root)
	}
	base, err := s.Lookup("PublisherInfoBase")
	if err != nil || !base.Abstract {
		t.Fatalf("PublisherInfoBase: %+v %v", base, err)
	}
	am, err := s.Lookup("Publisher.ArbitrManager.v2")
	if err != nil || am.Base != "Publisher" {
		t.Fatalf("Publisher.ArbitrManager.v2: %+v %v", am, err)
	}
	chain, err := s.Lookup("ChangeEstimatesCurrentExpensesContent")
	if err != nil {
		t.Fatal(err)
	}
	if !s.DerivedFrom(chain.Name, "MessageContentBase") {
		t.Fatalf("expected chain to MessageContentBase, base=%q", chain.Base)
	}
	info, err := s.Lookup("Messages.MessageData.PublisherInfo")
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Elems) == 0 || len(info.Elems[0].Choice) < 8 {
		t.Fatalf("PublisherInfo choice: %+v", info.Elems)
	}
	if len(s.Types) < 80 {
		t.Fatalf("types %d, want many named/inline types", len(s.Types))
	}
}

func TestTemplateRoundTrip(t *testing.T) {
	s := loadMessages(t)
	raw, err := EncodeTemplate(s)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseTemplate(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Root != s.Root {
		t.Fatalf("root %q", got.Root)
	}
	am, err := got.Lookup("Messages.MessageData.PublisherInfo.ArbitrManager")
	if err != nil || am.Base != "PublisherInfoBase" {
		t.Fatalf("arbitr: %+v %v", am, err)
	}
	info, err := got.Lookup("Messages.MessageData.PublisherInfo")
	if err != nil || len(info.Elems) != 1 || len(info.Elems[0].Choice) != 2 {
		t.Fatalf("choice: %+v %v", info, err)
	}
}

func TestXMLJSONInheritanceRoundTrip(t *testing.T) {
	s := loadMessages(t)
	xmlSrc, err := os.ReadFile(filepath.Join("testdata", "messages.xml"))
	if err != nil {
		t.Fatal(err)
	}
	inst, err := s.DecodeXML(xmlSrc)
	if err != nil {
		t.Fatal(err)
	}
	msg, ok := inst.Value.(map[string]any)
	if !ok {
		t.Fatalf("root value %T", inst.Value)
	}
	data := first(msg["MessageData"])
	pubInfo := mustMap(t, data["PublisherInfo"])
	am := mustMap(t, pubInfo["ArbitrManager"])
	if am["@Id"] != int64(7) || am["@FirstName"] != "Ada" || am["OGRN"] != "1027700000000" {
		t.Fatalf("choice+extension: %#v", am)
	}
	publisher := mustMap(t, data["Publisher"])
	if publisher["$type"] != "Publisher.ArbitrManager.v2" || publisher["Inn"] != "7700000000" {
		t.Fatalf("xsi:type: %#v", publisher)
	}
	info := mustMap(t, data["MessageInfo"])
	court := mustMap(t, info["CourtDecision"])
	if court["Text"] != "base text" {
		t.Fatalf("inherited Text: %#v", court)
	}
	if court["NextCourtSessionDate"] != nil {
		t.Fatalf("nillable: %#v", court["NextCourtSessionDate"])
	}

	js, err := inst.EncodeJSON()
	if err != nil {
		t.Fatal(err)
	}
	back, err := s.DecodeJSON(js)
	if err != nil {
		t.Fatal(err)
	}
	xmlOut, err := back.EncodeXML()
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.DecodeXML(xmlOut)
	if err != nil {
		t.Fatal(err)
	}
	data = first(mustMap(t, again.Value)["MessageData"])
	am = mustMap(t, mustMap(t, data["PublisherInfo"])["ArbitrManager"])
	if am["@Id"] != int64(7) || am["OGRN"] != "1027700000000" {
		t.Fatalf("round-trip: %#v", am)
	}
	publisher = mustMap(t, data["Publisher"])
	if publisher["$type"] != "Publisher.ArbitrManager.v2" {
		t.Fatalf("round-trip xsi:type: %#v", publisher)
	}
}

func hasField(fields []Field, name string) bool {
	for _, f := range fields {
		if f.Name == name {
			return true
		}
	}
	return false
}

func mustMap(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("want map, got %T %#v", v, v)
	}
	return m
}

func first(v any) map[string]any {
	switch x := v.(type) {
	case []any:
		if len(x) == 0 {
			return nil
		}
		m, _ := x[0].(map[string]any)
		return m
	case map[string]any:
		return x
	default:
		return nil
	}
}

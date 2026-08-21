package api

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"typeconverter/config"
	"typeconverter/model"
	"typeconverter/queue"
)

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	s := NewServerWith(queue.Config{Workers: 2, QueueSize: 64})
	t.Cleanup(s.Close)
	return s.Handler()
}

func TestXMLSendAndReceive(t *testing.T) {
	h := newTestHandler(t)

	src := []byte(`<catalog generated="2026-01-01"><book id="42"><title>urgent</title></book></catalog>`)
	id := enqueue(t, h, http.MethodPost, "/xml", "application/xml", src)
	rec := waitJobXML(t, h, id)

	got, err := model.DecodeXML(rec.Body.Bytes())
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Generated != "2026-01-01" || len(got.Books) != 1 || got.Books[0].ID != 42 {
		t.Fatalf("unexpected catalog: %+v", got)
	}

	rec = doRequest(t, h, http.MethodGet, "/xml", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /xml status %d body %s", rec.Code, rec.Body.Bytes())
	}
	stored, err := model.DecodeXML(rec.Body.Bytes())
	if err != nil {
		t.Fatalf("decode stored xml: %v", err)
	}
	if stored.Books[0].Title != "urgent" {
		t.Fatalf("stored xml: %+v", stored)
	}
}

func TestJSONSendAndReceive(t *testing.T) {
	h := newTestHandler(t)

	src := []byte(`{"generated":"2026-01-01","books":[{"id":42,"title":"urgent"}]}`)
	id := enqueue(t, h, http.MethodPost, "/json", "application/json", src)
	rec := waitJobJSON(t, h, id)

	got, err := model.DecodeJSON(rec.Body.Bytes())
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Generated != "2026-01-01" || len(got.Books) != 1 || got.Books[0].ID != 42 {
		t.Fatalf("unexpected catalog: %+v", got)
	}

	rec = doRequest(t, h, http.MethodGet, "/json", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /json status %d body %s", rec.Code, rec.Body.Bytes())
	}
	stored, err := model.DecodeJSON(rec.Body.Bytes())
	if err != nil {
		t.Fatalf("stored json: %v", err)
	}
	if stored.Books[0].Title != "urgent" {
		t.Fatalf("stored json: %+v", stored)
	}
}

func TestXMLAndJSONShareDocument(t *testing.T) {
	h := newTestHandler(t)

	xmlSrc := []byte(`<catalog generated="2026-01-01"><book id="7"><title>Ada</title></book></catalog>`)
	id := enqueue(t, h, http.MethodPost, "/xml", "application/xml", xmlSrc)
	waitJobXML(t, h, id)

	jsonRec := doRequest(t, h, http.MethodGet, "/json", "", nil)
	if jsonRec.Code != http.StatusOK {
		t.Fatalf("GET /json after xml: %d %s", jsonRec.Code, jsonRec.Body.Bytes())
	}
	fromXML, err := model.DecodeJSON(jsonRec.Body.Bytes())
	if err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if fromXML.Generated != "2026-01-01" || fromXML.Books[0].Title != "Ada" {
		t.Fatalf("xml->json: %+v", fromXML)
	}

	jsonSrc := []byte(`{"generated":"2026-02-02","books":[{"id":8,"title":"Go"}]}`)
	id = enqueue(t, h, http.MethodPost, "/json", "application/json", jsonSrc)
	waitJobJSON(t, h, id)

	gotXML := doRequest(t, h, http.MethodGet, "/xml", "", nil)
	if gotXML.Code != http.StatusOK {
		t.Fatalf("GET /xml after json: %d %s", gotXML.Code, gotXML.Body.Bytes())
	}
	fromJSON, err := model.DecodeXML(gotXML.Body.Bytes())
	if err != nil {
		t.Fatalf("decode xml: %v", err)
	}
	if fromJSON.Generated != "2026-02-02" || fromJSON.Books[0].Title != "Go" {
		t.Fatalf("json->xml: %+v", fromJSON)
	}
}

func TestGetBeforePostNotFound(t *testing.T) {
	h := newTestHandler(t)

	xmlRec := doRequest(t, h, http.MethodGet, "/xml", "", nil)
	if xmlRec.Code != http.StatusNotFound {
		t.Fatalf("GET /xml: %d", xmlRec.Code)
	}
	jsonRec := doRequest(t, h, http.MethodGet, "/json", "", nil)
	if jsonRec.Code != http.StatusNotFound {
		t.Fatalf("GET /json: %d", jsonRec.Code)
	}
}

func TestInvalidBodies(t *testing.T) {
	h := newTestHandler(t)

	xmlID := enqueue(t, h, http.MethodPost, "/xml", "application/xml", []byte(`<a></b>`))
	xmlRec := waitJobResult(t, h, xmlID, "/xml")
	if xmlRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid xml: %d %s", xmlRec.Code, xmlRec.Body.Bytes())
	}

	jsonID := enqueue(t, h, http.MethodPost, "/json", "application/json", []byte(`{`))
	jsonRec := waitJobResult(t, h, jsonID, "/json")
	if jsonRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid json: %d %s", jsonRec.Code, jsonRec.Body.Bytes())
	}

	wrongID := enqueue(t, h, http.MethodPost, "/xml", "application/xml", []byte(`<order/>`))
	wrongRoot := waitJobResult(t, h, wrongID, "/xml")
	if wrongRoot.Code != http.StatusUnprocessableEntity {
		t.Fatalf("wrong xml root: %d %s", wrongRoot.Code, wrongRoot.Body.Bytes())
	}
}

func TestUnsupportedContentType(t *testing.T) {
	h := newTestHandler(t)

	xmlRec := doRequest(t, h, http.MethodPost, "/xml", "application/json", []byte(`<catalog/>`))
	if xmlRec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("xml ct: %d", xmlRec.Code)
	}
	jsonRec := doRequest(t, h, http.MethodPost, "/json", "application/xml", []byte(`{}`))
	if jsonRec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("json ct: %d", jsonRec.Code)
	}
}

func TestEmptyBody(t *testing.T) {
	h := newTestHandler(t)

	xmlRec := doRequest(t, h, http.MethodPost, "/xml", "application/xml", []byte("   "))
	if xmlRec.Code != http.StatusBadRequest {
		t.Fatalf("empty xml: %d", xmlRec.Code)
	}
	jsonRec := doRequest(t, h, http.MethodPost, "/json", "application/json", nil)
	if jsonRec.Code != http.StatusBadRequest {
		t.Fatalf("empty json: %d", jsonRec.Code)
	}
}

func TestCatalogFilesRoundTrip(t *testing.T) {
	h := newTestHandler(t)

	xmlSrc, err := os.ReadFile(filepath.Join("..", "xmlconv", "testdata", "catalog.xml"))
	if err != nil {
		t.Fatal(err)
	}
	id := enqueue(t, h, http.MethodPost, "/xml", "text/xml", xmlSrc)
	xmlRec := waitJobXML(t, h, id)
	fromXML, err := model.DecodeXML(xmlRec.Body.Bytes())
	if err != nil {
		t.Fatalf("decode catalog xml: %v", err)
	}
	if fromXML.Generated != "2026-01-01" || len(fromXML.Books) != 3 || fromXML.Books[0].Title != "Go in Action" {
		t.Fatalf("catalog xml: %+v", fromXML)
	}

	jsonGet := doRequest(t, h, http.MethodGet, "/json", "", nil)
	if jsonGet.Code != http.StatusOK {
		t.Fatalf("GET /json: %d %s", jsonGet.Code, jsonGet.Body.Bytes())
	}
	fromJSON, err := model.DecodeJSON(jsonGet.Body.Bytes())
	if err != nil {
		t.Fatalf("decode catalog json: %v", err)
	}
	if fromJSON.Books[1].Meta == nil || fromJSON.Books[1].Meta.Author != "Ann" {
		t.Fatalf("catalog json: %+v", fromJSON)
	}
}

func TestQueueFullReturns503(t *testing.T) {
	started := make(chan struct{})
	block := make(chan struct{})
	var once sync.Once
	s := NewServerWith(queue.Config{Workers: 1, QueueSize: 1})
	t.Cleanup(func() {
		close(block)
		s.Close()
	})
	s.q.Close()
	s.q = queue.New(queue.Config{Workers: 1, QueueSize: 1}, func(queue.Format, []byte) (queue.Document, error) {
		once.Do(func() { close(started) })
		<-block
		return &model.Catalog{Title: "ok"}, nil
	}, s.onJobDone)

	h := s.Handler()
	first := doRequest(t, h, http.MethodPost, "/json", "application/json", []byte(`{"title":"a"}`))
	if first.Code != http.StatusAccepted {
		t.Fatalf("first: %d %s", first.Code, first.Body.Bytes())
	}
	<-started
	second := doRequest(t, h, http.MethodPost, "/json", "application/json", []byte(`{"title":"b"}`))
	if second.Code != http.StatusAccepted {
		t.Fatalf("second: %d %s", second.Code, second.Body.Bytes())
	}
	third := doRequest(t, h, http.MethodPost, "/json", "application/json", []byte(`{"title":"c"}`))
	if third.Code != http.StatusServiceUnavailable {
		t.Fatalf("third: %d %s", third.Code, third.Body.Bytes())
	}
}

func TestUnknownJob(t *testing.T) {
	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodGet, "/jobs/missing", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: %d %s", rec.Code, rec.Body.Bytes())
	}
}

func TestListJobs(t *testing.T) {
	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodGet, "/jobs", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("empty list: %d %s", rec.Code, rec.Body.Bytes())
	}
	if rec.Body.String() != "[]" {
		t.Fatalf("empty list body %s", rec.Body.Bytes())
	}

	id := enqueue(t, h, http.MethodPost, "/json", "application/json", []byte(`{"title":"listed"}`))
	waitJobJSON(t, h, id)

	rec = doRequest(t, h, http.MethodGet, "/jobs", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.Bytes())
	}
	var jobs []jobStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &jobs); err != nil {
		t.Fatalf("list json: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != id || jobs[0].Status != "done" || jobs[0].Kind != queue.KindDocument {
		t.Fatalf("jobs: %+v", jobs)
	}

	cleared := doRequest(t, h, http.MethodDelete, "/jobs", "", nil)
	if cleared.Code != http.StatusOK {
		t.Fatalf("DELETE /jobs: %d %s", cleared.Code, cleared.Body.Bytes())
	}
	rec = doRequest(t, h, http.MethodGet, "/jobs", "", nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "[]" {
		t.Fatalf("cleared list: %d %s", rec.Code, rec.Body.Bytes())
	}
}

func TestUIIndex(t *testing.T) {
	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodGet, "/", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /: %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type: %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "TypeConverter") {
		t.Fatalf("index body: %s", rec.Body.Bytes())
	}
	if !strings.Contains(body, `class="hero"`) || !strings.Contains(body, "class=\"lede\"") {
		t.Fatal("index is missing centered hero heading")
	}
	if !strings.Contains(body, "Одна модель") || !strings.Contains(body, "✨") || !strings.Contains(body, "lede-row") {
		t.Fatal("index is missing updated hero description")
	}
	if !strings.Contains(body, "XML ⇄ JSON") || !strings.Contains(body, "XSD / шаблон") {
		t.Fatalf("index is missing conversion windows: %s", rec.Body.Bytes())
	}
	if strings.Contains(body, "data-schema-out") || strings.Contains(body, "data-out=") {
		t.Fatal("UI should not toggle converting a format into itself")
	}
	if strings.Contains(body, "id=\"load-sample\"") || strings.Contains(body, "id=\"schema-sample\"") {
		t.Fatal("sample buttons should be replaced by format switching")
	}
	if !strings.Contains(body, "id=\"clear-jobs\"") || !strings.Contains(body, "id=\"schema-jobs\"") || !strings.Contains(body, "id=\"schema-stages\"") {
		t.Fatal("index is missing history clear")
	}
	if !strings.Contains(body, `id="copy-out" disabled`) || !strings.Contains(body, `id="schema-copy" disabled`) {
		t.Fatal("copy/download should start disabled")
	}

	css := doRequest(t, h, http.MethodGet, "/app.css", "", nil)
	if css.Code != http.StatusOK || !strings.Contains(css.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("css: %d %s", css.Code, css.Header().Get("Content-Type"))
	}
	if !strings.Contains(css.Body.String(), "--display") || !strings.Contains(css.Body.String(), "Unbounded") {
		t.Fatal("app.css should use a display font")
	}
	js := doRequest(t, h, http.MethodGet, "/app.js", "", nil)
	if js.Code != http.StatusOK || !strings.Contains(js.Header().Get("Content-Type"), "javascript") {
		t.Fatalf("js: %d %s", js.Code, js.Header().Get("Content-Type"))
	}
	jsBody := js.Body.String()
	if !strings.Contains(jsBody, "const STATUS_LABEL") {
		t.Fatal("app.js is missing STATUS_LABEL")
	}
	if !strings.Contains(jsBody, "SAMPLE_XSD") || !strings.Contains(jsBody, "parseSchema") {
		t.Fatal("app.js is missing schema window")
	}
	if strings.Count(jsBody, "function prettyJSON") != 1 {
		t.Fatal("app.js should define prettyJSON once")
	}
	if !strings.Contains(jsBody, "function clipPreview") || !strings.Contains(jsBody, "PREVIEW_LINES") {
		t.Fatal("app.js should clip bulky previews by line count")
	}
	if !strings.Contains(jsBody, "clipPreview(formatBody(format, text))") {
		t.Fatal("app.js should pretty-print results for reading")
	}
	if !strings.Contains(jsBody, `return text.slice(0, PREVIEW_LIMIT) + "…"`) {
		t.Fatal("app.js should truncate bulky text with an ellipsis")
	}
	if !strings.Contains(jsBody, "function download(text, format, name)") || !strings.Contains(jsBody, "formatBody(format, text") {
		t.Fatal("app.js downloads should pretty-print the file")
	}
	if !strings.Contains(jsBody, "function convertTarget") {
		t.Fatal("app.js should convert XML to JSON and JSON to XML only")
	}
	if !strings.Contains(jsBody, "function requiredExts") || !strings.Contains(jsBody, "fileMatchesFormat") {
		t.Fatal("app.js should check file extensions")
	}
	if !strings.Contains(jsBody, `".jsont"`) || !strings.Contains(jsBody, `".jsontemplate"`) {
		t.Fatal("app.js should accept .jsont and .jsontemplate for JSON template files")
	}
	if !strings.Contains(body, `accept=".xml,application/xml,text/xml"`) {
		t.Fatal("convert file input should default to XML")
	}
	if !strings.Contains(body, `accept=".xsd,application/xml,text/xml,application/xsd+xml"`) {
		t.Fatal("schema file input should default to XSD")
	}
	if !strings.Contains(jsBody, "?catalog=1") {
		t.Fatal("app.js should convert documents as catalog")
	}
	if !strings.Contains(jsBody, "loadSample()") || !strings.Contains(jsBody, "loadSchemaSample()") {
		t.Fatal("app.js should apply samples when switching formats")
	}
	if strings.Contains(jsBody, "Ctrl+Enter") || strings.Contains(jsBody, "key === \"Enter\"") {
		t.Fatal("conversion should run only from the button, not from keyboard shortcuts")
	}
	if !strings.Contains(jsBody, "function resetConvertOutput") || !strings.Contains(jsBody, "function resetSchemaOutput") {
		t.Fatal("switching format should clear results until the convert button is pressed")
	}
	if !strings.Contains(jsBody, "function clearJobs") || !strings.Contains(jsBody, "async function clearSchema") {
		t.Fatal("app.js is missing history or schema clear")
	}
	if !strings.Contains(jsBody, "/jobs?kind=schema") || !strings.Contains(jsBody, "/jobs?kind=document") {
		t.Fatal("app.js should clear schema and document jobs separately")
	}
}

func TestSQLiteSurvivesRestart(t *testing.T) {
	cfg := config.Default()
	cfg.Store.Path = filepath.Join(t.TempDir(), "typeconverter.db")
	cfg.Queue.Workers = 2
	cfg.Queue.QueueSize = 16

	s1 := NewFromConfig(cfg)
	h := s1.Handler()
	id := enqueue(t, h, http.MethodPost, "/json?catalog=1", "application/json", []byte(`{"title":"keep-me","books":[{"id":1,"title":"Go"}]}`))
	waitJobJSON(t, h, id)
	s1.Close()

	s2 := NewFromConfig(cfg)
	t.Cleanup(s2.Close)
	h = s2.Handler()

	xmlRec := doRequest(t, h, http.MethodGet, "/xml", "", nil)
	if xmlRec.Code != http.StatusOK {
		t.Fatalf("GET /xml after restart: %d %s", xmlRec.Code, xmlRec.Body.Bytes())
	}
	got, err := model.DecodeXML(xmlRec.Body.Bytes())
	if err != nil || got.Title != "keep-me" {
		t.Fatalf("stored xml after restart: %+v %v", got, err)
	}
	jsonRec := doRequest(t, h, http.MethodGet, "/jobs/"+id+"/json", "", nil)
	if jsonRec.Code != http.StatusOK {
		t.Fatalf("job json after restart: %d %s", jsonRec.Code, jsonRec.Body.Bytes())
	}
	listed := doRequest(t, h, http.MethodGet, "/jobs", "", nil)
	var jobs []jobStatus
	if err := json.Unmarshal(listed.Body.Bytes(), &jobs); err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != id || jobs[0].Status != "done" {
		t.Fatalf("jobs after restart: %+v", jobs)
	}
}

func TestSchemaXSDAndJSONTemplate(t *testing.T) {
	s := NewServerWith(queue.Config{Workers: 2, QueueSize: 64})
	t.Cleanup(s.Close)
	h := s.Handler()

	xsd, err := os.ReadFile(filepath.Join("..", "schema", "testdata", "messages.xsd"))
	if err != nil {
		t.Fatal(err)
	}
	id := enqueue(t, h, http.MethodPost, "/xsd", "application/xml", xsd)
	rec := waitJobJSON(t, h, id)
	var tpl schemaTemplate
	if err := json.Unmarshal(rec.Body.Bytes(), &tpl); err != nil {
		t.Fatalf("template: %v", err)
	}
	if tpl.Root.Name != "Messages" {
		t.Fatalf("template root %+v", tpl.Root)
	}

	xmlSrc, err := os.ReadFile(filepath.Join("..", "schema", "testdata", "messages.xml"))
	if err != nil {
		t.Fatal(err)
	}
	id = enqueue(t, h, http.MethodPost, "/xml", "application/xml", xmlSrc)
	jsonRec := waitJobJSON(t, h, id)
	var doc map[string]any
	if err := json.Unmarshal(jsonRec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("job json: %v %s", err, jsonRec.Body.Bytes())
	}
	messages, _ := doc["Messages"].(map[string]any)
	list, _ := messages["MessageData"].([]any)
	if len(list) != 1 {
		t.Fatalf("MessageData: %#v", doc)
	}
	row, _ := list[0].(map[string]any)
	info, _ := row["PublisherInfo"].(map[string]any)
	am, _ := info["ArbitrManager"].(map[string]any)
	if am["OGRN"] != "1027700000000" {
		t.Fatalf("choice extension: %#v", am)
	}
	pub, _ := row["Publisher"].(map[string]any)
	if pub["$type"] != "Publisher.ArbitrManager.v2" {
		t.Fatalf("xsi:type: %#v", pub)
	}

	tplRec := doRequest(t, h, http.MethodGet, "/jsontemplate", "", nil)
	if tplRec.Code != http.StatusOK {
		t.Fatalf("GET /jsontemplate: %d", tplRec.Code)
	}
	infoOK := doRequest(t, h, http.MethodGet, "/schema", "", nil)
	if infoOK.Code != http.StatusOK || !strings.Contains(infoOK.Body.String(), `"root":"Messages"`) {
		t.Fatalf("GET /schema: %d %s", infoOK.Code, infoOK.Body.Bytes())
	}
	clear := doRequest(t, h, http.MethodDelete, "/schema", "", nil)
	if clear.Code != http.StatusOK {
		t.Fatalf("DELETE /schema: %d", clear.Code)
	}
	infoGone := doRequest(t, h, http.MethodGet, "/schema", "", nil)
	if infoGone.Code != http.StatusNotFound {
		t.Fatalf("GET /schema after clear: %d", infoGone.Code)
	}
	missing := doRequest(t, h, http.MethodGet, "/jsontemplate", "", nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("cleared schema: %d", missing.Code)
	}
}

func TestSchemaJobsUseQueue(t *testing.T) {
	s := NewServerWith(queue.Config{Workers: 2, QueueSize: 64})
	t.Cleanup(s.Close)
	h := s.Handler()

	xsd, err := os.ReadFile(filepath.Join("..", "schema", "testdata", "messages.xsd"))
	if err != nil {
		t.Fatal(err)
	}
	xsdID := enqueue(t, h, http.MethodPost, "/xsd", "application/xml", xsd)
	tplRec := waitJobJSON(t, h, xsdID)
	var tpl schemaTemplate
	if err := json.Unmarshal(tplRec.Body.Bytes(), &tpl); err != nil {
		t.Fatalf("template: %v", err)
	}
	if tpl.Root.Name != "Messages" {
		t.Fatalf("template root %+v", tpl.Root)
	}

	listed := doRequest(t, h, http.MethodGet, "/jobs", "", nil)
	var jobs []jobStatus
	if err := json.Unmarshal(listed.Body.Bytes(), &jobs); err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != xsdID || jobs[0].Kind != queue.KindSchema || jobs[0].Format != "xsd" {
		t.Fatalf("schema jobs: %+v", jobs)
	}

	tplID := enqueue(t, h, http.MethodPost, "/jsontemplate", "application/json", tplRec.Body.Bytes())
	xsdOut := waitJobXML(t, h, tplID)
	if xsdOut.Code != http.StatusOK || !strings.Contains(xsdOut.Body.String(), "schema") {
		t.Fatalf("POST /jsontemplate xml: %d %s", xsdOut.Code, xsdOut.Body.Bytes())
	}

	badID := enqueue(t, h, http.MethodPost, "/xsd", "application/xml", []byte(`<schema/>`))
	fail := waitJobResult(t, h, badID, "/xml")
	if fail.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid xsd: %d %s", fail.Code, fail.Body.Bytes())
	}

	docID := enqueue(t, h, http.MethodPost, "/json", "application/json", []byte(`{"title":"keep"}`))
	waitJobJSON(t, h, docID)

	cleared := doRequest(t, h, http.MethodDelete, "/jobs?kind=schema", "", nil)
	if cleared.Code != http.StatusOK {
		t.Fatalf("DELETE kind=schema: %d", cleared.Code)
	}
	listed = doRequest(t, h, http.MethodGet, "/jobs", "", nil)
	if err := json.Unmarshal(listed.Body.Bytes(), &jobs); err != nil {
		t.Fatalf("list after schema clear: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != docID || jobs[0].Kind != queue.KindDocument {
		t.Fatalf("document job should remain: %+v", jobs)
	}
}

func TestCatalogQueryIgnoresLoadedSchema(t *testing.T) {
	s := NewServerWith(queue.Config{Workers: 2, QueueSize: 64})
	t.Cleanup(s.Close)
	h := s.Handler()

	xsd, err := os.ReadFile(filepath.Join("..", "schema", "testdata", "messages.xsd"))
	if err != nil {
		t.Fatal(err)
	}
	id := enqueue(t, h, http.MethodPost, "/xsd", "application/xml", xsd)
	waitJobJSON(t, h, id)

	catalogXML := []byte(`<catalog generated="2026-01-01"><book id="42"><title>urgent</title></book></catalog>`)
	blocked := enqueue(t, h, http.MethodPost, "/xml", "application/xml", catalogXML)
	fail := waitJobResult(t, h, blocked, "/xml")
	if fail.Code != http.StatusUnprocessableEntity {
		t.Fatalf("catalog xml against schema: %d %s", fail.Code, fail.Body.Bytes())
	}

	id = enqueue(t, h, http.MethodPost, "/xml?catalog=1", "application/xml", catalogXML)
	ok := waitJobXML(t, h, id)
	got, err := model.DecodeXML(ok.Body.Bytes())
	if err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	if got.Generated != "2026-01-01" || len(got.Books) != 1 || got.Books[0].Title != "urgent" {
		t.Fatalf("catalog: %+v", got)
	}
}

type schemaTemplate struct {
	Root struct {
		Name string `json:"name"`
	} `json:"root"`
}

func enqueue(t *testing.T, h http.Handler, method, path, contentType string, body []byte) string {
	t.Helper()
	rec := doRequest(t, h, method, path, contentType, body)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("%s %s status %d body %s", method, path, rec.Code, rec.Body.Bytes())
	}
	id := jobIDFrom(t, rec)
	if loc := rec.Header().Get("Location"); loc != "/jobs/"+id {
		t.Fatalf("location: %q", loc)
	}
	return id
}

func waitJobXML(t *testing.T, h http.Handler, id string) *httptest.ResponseRecorder {
	t.Helper()
	return waitReady(t, h, "/jobs/"+id+"/xml")
}

func waitJobJSON(t *testing.T, h http.Handler, id string) *httptest.ResponseRecorder {
	t.Helper()
	return waitReady(t, h, "/jobs/"+id+"/json")
}

func waitJobResult(t *testing.T, h http.Handler, id, format string) *httptest.ResponseRecorder {
	t.Helper()
	return waitReady(t, h, "/jobs/"+id+format)
}

func waitReady(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var rec *httptest.ResponseRecorder
	for time.Now().Before(deadline) {
		rec = doRequest(t, h, http.MethodGet, path, "", nil)
		if rec.Code != http.StatusConflict {
			return rec
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s: %d %s", path, rec.Code, rec.Body.Bytes())
	return rec
}

func jobIDFrom(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	ct := rec.Header().Get("Content-Type")
	if strings.Contains(ct, "json") {
		var ticket jobTicket
		if err := json.Unmarshal(rec.Body.Bytes(), &ticket); err != nil {
			t.Fatalf("ticket json: %v body %s", err, rec.Body.Bytes())
		}
		if ticket.ID == "" {
			t.Fatalf("empty job id: %s", rec.Body.Bytes())
		}
		return ticket.ID
	}
	var ticket jobTicket
	if err := xml.Unmarshal(rec.Body.Bytes(), &ticket); err != nil {
		t.Fatalf("ticket xml: %v body %s", err, rec.Body.Bytes())
	}
	if ticket.ID == "" {
		t.Fatalf("empty job id: %s", rec.Body.Bytes())
	}
	return ticket.ID
}

func doRequest(t *testing.T, h http.Handler, method, path, contentType string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

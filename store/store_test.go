package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestPutGetListDelete(t *testing.T) {
	st := openTemp(t)

	rec := Record{
		ID:        "abc",
		Kind:      "document",
		Format:    "xml",
		Status:    "done",
		XML:       []byte("<catalog/>"),
		JSON:      []byte(`{"title":"x"}`),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := st.Put(rec); err != nil {
		t.Fatal(err)
	}
	got, ok, err := st.Get("abc")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if string(got.XML) != "<catalog/>" || string(got.JSON) != `{"title":"x"}` || got.Kind != "document" {
		t.Fatalf("record %+v", got)
	}
	list, err := st.List()
	if err != nil || len(list) != 1 || list[0].ID != "abc" {
		t.Fatalf("list: %+v %v", list, err)
	}
	if err := st.DeleteFinished("document"); err != nil {
		t.Fatal(err)
	}
	if list, err = st.List(); err != nil || len(list) != 0 {
		t.Fatalf("cleared: %+v %v", list, err)
	}
}

func TestCurrentDocumentAndSchema(t *testing.T) {
	st := openTemp(t)
	if err := st.SetDocument([]byte("<a/>"), []byte(`{"a":1}`)); err != nil {
		t.Fatal(err)
	}
	xmlBody, jsonBody, ok, err := st.Document()
	if err != nil || !ok || string(xmlBody) != "<a/>" || string(jsonBody) != `{"a":1}` {
		t.Fatalf("doc: %q %q ok=%v err=%v", xmlBody, jsonBody, ok, err)
	}
	if err := st.SetSchema("Messages", []byte("<xs/>"), []byte(`{"root":{}}`)); err != nil {
		t.Fatal(err)
	}
	root, xsd, _, ok, err := st.Schema()
	if err != nil || !ok || root != "Messages" || string(xsd) != "<xs/>" {
		t.Fatalf("schema: %q %q ok=%v err=%v", root, xsd, ok, err)
	}
	if err := st.ClearDocument(); err != nil {
		t.Fatal(err)
	}
	if _, _, ok, err = st.Document(); err != nil || ok {
		t.Fatalf("cleared doc ok=%v err=%v", ok, err)
	}
	if err := st.ClearSchema(); err != nil {
		t.Fatal(err)
	}
	if _, _, _, ok, err = st.Schema(); err != nil || ok {
		t.Fatalf("cleared schema ok=%v err=%v", ok, err)
	}
}

func TestReopenKeepsBlobs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jobs.db")
	st, err := Open(path, 100)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Put(Record{ID: "1", Kind: "document", Format: "json", Status: "done", JSON: []byte(`{"ok":true}`)}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetDocument([]byte("<c/>"), []byte(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st, err = Open(path, 100)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	got, ok, err := st.Get("1")
	if err != nil || !ok || string(got.JSON) != `{"ok":true}` {
		t.Fatalf("reopen job: %+v ok=%v err=%v", got, ok, err)
	}
	_, jsonBody, ok, err := st.Document()
	if err != nil || !ok || string(jsonBody) != `{"ok":true}` {
		t.Fatalf("reopen doc: %q ok=%v err=%v", jsonBody, ok, err)
	}
}

func TestInterruptedJobsFailOnOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jobs.db")
	st, err := Open(path, 100)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Put(Record{ID: "run", Kind: "document", Format: "xml", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = Open(path, 100)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	got, ok, err := st.Get("run")
	if err != nil || !ok || got.Status != "failed" || got.Error != "interrupted" {
		t.Fatalf("interrupted: %+v ok=%v err=%v", got, ok, err)
	}
}

func TestGCRemovesOldestFinished(t *testing.T) {
	st, err := Open("", 2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	now := time.Now()
	for i, id := range []string{"a", "b", "c"} {
		if err := st.Put(Record{
			ID:        id,
			Kind:      "document",
			Format:    "json",
			Status:    "done",
			CreatedAt: now.Add(time.Duration(i) * time.Millisecond),
			UpdatedAt: now.Add(time.Duration(i) * time.Millisecond),
		}); err != nil {
			t.Fatal(err)
		}
	}
	list, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("len %d", len(list))
	}
	ids := map[string]bool{list[0].ID: true, list[1].ID: true}
	if ids["a"] || !ids["b"] || !ids["c"] {
		t.Fatalf("kept %+v", list)
	}
}

func openTemp(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "t.db"), 100)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

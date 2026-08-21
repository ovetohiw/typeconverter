package queue

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"typeconverter/model"
)

func TestSubmitAndWaitXML(t *testing.T) {
	q := New(Config{Workers: 2, QueueSize: 8}, ProcessCatalog, nil)
	t.Cleanup(q.Close)

	job, err := q.Submit(FormatXML, []byte(`<catalog generated="2026-01-01"><book id="1"><title>Go</title></book></catalog>`))
	if err != nil {
		t.Fatal(err)
	}
	got, ok := q.Wait(job.ID, 2*time.Second)
	if !ok || got.Status != StatusDone {
		t.Fatalf("job: %+v ok=%v", got, ok)
	}
	if got.Catalog == nil || got.Catalog.Books[0].Title != "Go" {
		t.Fatalf("catalog: %+v", got.Catalog)
	}
}

func TestSubmitJSONAndCallback(t *testing.T) {
	var got atomic.Pointer[model.Catalog]
	q := New(Config{Workers: 1, QueueSize: 4}, ProcessCatalog, func(job *Job) {
		got.Store(job.Catalog)
	})
	t.Cleanup(q.Close)

	job, err := q.Submit(FormatJSON, []byte(`{"generated":"2026-02-02","books":[{"id":8,"title":"Ada"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	done, ok := q.Wait(job.ID, 2*time.Second)
	if !ok || done.Status != StatusDone {
		t.Fatalf("job: %+v", done)
	}
	cat := got.Load()
	if cat == nil || cat.Books[0].Title != "Ada" {
		t.Fatalf("callback: %+v", cat)
	}
}

func TestFailedJob(t *testing.T) {
	q := New(Config{Workers: 1, QueueSize: 4}, ProcessCatalog, nil)
	t.Cleanup(q.Close)

	job, err := q.Submit(FormatXML, []byte(`<a></b>`))
	if err != nil {
		t.Fatal(err)
	}
	got, ok := q.Wait(job.ID, 2*time.Second)
	if !ok || got.Status != StatusFailed || got.Error == "" {
		t.Fatalf("expected failed job, got %+v", got)
	}
}

func TestQueueFull(t *testing.T) {
	started := make(chan struct{})
	block := make(chan struct{})
	var once sync.Once
	q := New(Config{Workers: 1, QueueSize: 1}, func(Format, []byte) (Document, error) {
		once.Do(func() { close(started) })
		<-block
		return &model.Catalog{Title: "ok"}, nil
	}, nil)
	t.Cleanup(func() {
		close(block)
		q.Close()
	})

	if _, err := q.Submit(FormatJSON, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	<-started
	if _, err := q.Submit(FormatJSON, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Submit(FormatJSON, []byte(`{}`)); err != ErrQueueFull {
		t.Fatalf("got %v, want ErrQueueFull", err)
	}
}

func TestParallelSubmit(t *testing.T) {
	q := New(Config{Workers: 4, QueueSize: 64}, ProcessCatalog, nil)
	t.Cleanup(q.Close)

	const n = 32
	var wg sync.WaitGroup
	wg.Add(n)
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			job, err := q.Submit(FormatJSON, []byte(`{"title":"x","books":[{"id":1,"title":"t"}]}`))
			if err != nil {
				errCh <- err
				return
			}
			got, ok := q.Wait(job.ID, 2*time.Second)
			if !ok || got.Status != StatusDone {
				errCh <- errString("job not done")
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
}

func TestListNewestFirst(t *testing.T) {
	q := New(Config{Workers: 1, QueueSize: 8}, ProcessCatalog, nil)
	t.Cleanup(q.Close)

	first, err := q.Submit(FormatJSON, []byte(`{"title":"a"}`))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	second, err := q.Submit(FormatJSON, []byte(`{"title":"b"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := q.Wait(first.ID, 2*time.Second); !ok {
		t.Fatal("wait first")
	}
	if _, ok := q.Wait(second.ID, 2*time.Second); !ok {
		t.Fatal("wait second")
	}
	list := q.List()
	if len(list) != 2 {
		t.Fatalf("len %d", len(list))
	}
	if list[0].ID != second.ID || list[1].ID != first.ID {
		t.Fatalf("order %+v", []string{list[0].ID, list[1].ID})
	}
	q.Clear()
	if got := q.List(); len(got) != 0 {
		t.Fatalf("cleared %d", len(got))
	}
}

func TestClearKind(t *testing.T) {
	q := New(Config{Workers: 1, QueueSize: 8}, func(format Format, body []byte) (Document, error) {
		if format == FormatXSD || format == FormatJSONTemplate {
			return &model.Catalog{Title: string(format)}, nil
		}
		return ProcessCatalog(format, body)
	}, nil)
	t.Cleanup(q.Close)

	doc, err := q.Submit(FormatJSON, []byte(`{"title":"doc"}`))
	if err != nil {
		t.Fatal(err)
	}
	sch, err := q.Submit(FormatXSD, []byte(`<schema/>`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := q.Wait(doc.ID, 2*time.Second); !ok {
		t.Fatal("wait doc")
	}
	if _, ok := q.Wait(sch.ID, 2*time.Second); !ok {
		t.Fatal("wait schema")
	}
	q.ClearKind(KindSchema)
	list := q.List()
	if len(list) != 1 || list[0].ID != doc.ID {
		t.Fatalf("after schema clear: %+v", list)
	}
	q.ClearKind(KindDocument)
	if got := q.List(); len(got) != 0 {
		t.Fatalf("after document clear: %d", len(got))
	}
}

func TestSubmitCatalogBypassesProcessor(t *testing.T) {
	q := New(Config{Workers: 1, QueueSize: 4}, func(Format, []byte) (Document, error) {
		t.Fatal("schema processor should not run")
		return nil, errString("unused")
	}, nil)
	t.Cleanup(q.Close)

	job, err := q.SubmitCatalog(FormatXML, []byte(`<catalog generated="2026-01-01"><book id="1"><title>Go</title></book></catalog>`))
	if err != nil {
		t.Fatal(err)
	}
	got, ok := q.Wait(job.ID, 2*time.Second)
	if !ok || got.Status != StatusDone || got.Catalog == nil || got.Catalog.Books[0].Title != "Go" {
		t.Fatalf("job: %+v ok=%v", got, ok)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

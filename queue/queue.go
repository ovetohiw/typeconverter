package queue

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"sync"
	"time"

	"typeconverter/model"
)

type Format string

const (
	FormatXML          Format = "xml"
	FormatJSON         Format = "json"
	FormatXSD          Format = "xsd"
	FormatJSONTemplate Format = "jsontemplate"
	KindDocument       string = "document"
	KindSchema         string = "schema"
)

type Status string

const (
	StatusQueued  Status = "queued"
	StatusRunning Status = "running"
	StatusDone    Status = "done"
	StatusFailed  Status = "failed"
)

var (
	ErrQueueFull = errors.New("queue is full")
	ErrClosed    = errors.New("queue is closed")
)

type Config struct {
	Workers   int
	QueueSize int
	MaxJobs   int
	Persist   Persist
}

// Persist keeps job metadata and encoded results. Optional: nil means memory only.
type Persist interface {
	Save(job Job, xmlBody, jsonBody []byte) error
	Delete(id string) error
	DeleteFinished(kind string) error
}

func DefaultConfig() Config {
	n := runtime.NumCPU()
	if n < 2 {
		n = 2
	}
	return Config{Workers: n, QueueSize: 256, MaxJobs: 10_000}
}

type Document interface {
	EncodeXML() ([]byte, error)
	EncodeJSON() ([]byte, error)
}

type Processor func(format Format, body []byte) (Document, error)

func ProcessCatalog(format Format, body []byte) (Document, error) {
	switch format {
	case FormatXML:
		cat, err := model.DecodeXML(body)
		if err != nil {
			return nil, err
		}
		return &cat, nil
	case FormatJSON:
		cat, err := model.DecodeJSON(body)
		if err != nil {
			return nil, err
		}
		return &cat, nil
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
}

type Job struct {
	ID        string
	Format    Format
	Status    Status
	Error     string
	Catalog   *model.Catalog
	Doc       Document
	XML       []byte
	JSON      []byte
	CreatedAt time.Time
	UpdatedAt time.Time
}

type task struct {
	id      string
	format  Format
	body    []byte
	catalog bool
}

type storedJob struct {
	Job
	done chan struct{}
}

type Queue struct {
	tasks   chan task
	proc    Processor
	onDone  func(*Job)
	persist Persist
	maxJobs int

	mu     sync.Mutex
	jobs   map[string]*storedJob
	closed bool
	wg     sync.WaitGroup
}

func New(cfg Config, proc Processor, onDone func(*Job)) *Queue {
	if cfg.Workers < 1 {
		cfg.Workers = 1
	}
	if cfg.QueueSize < 1 {
		cfg.QueueSize = 1
	}
	if cfg.MaxJobs < 1 {
		cfg.MaxJobs = 10_000
	}
	if proc == nil {
		proc = ProcessCatalog
	}
	q := &Queue{
		tasks:   make(chan task, cfg.QueueSize),
		proc:    proc,
		onDone:  onDone,
		persist: cfg.Persist,
		maxJobs: cfg.MaxJobs,
		jobs:    make(map[string]*storedJob),
	}
	q.wg.Add(cfg.Workers)
	for i := 0; i < cfg.Workers; i++ {
		go q.worker()
	}
	return q
}

func (q *Queue) Submit(format Format, body []byte) (Job, error) {
	return q.submit(format, body, false)
}

func (q *Queue) SubmitCatalog(format Format, body []byte) (Job, error) {
	return q.submit(format, body, true)
}

func (q *Queue) submit(format Format, body []byte, catalog bool) (Job, error) {
	id, err := newID()
	if err != nil {
		return Job{}, err
	}
	now := time.Now()
	st := &storedJob{
		Job: Job{
			ID:        id,
			Format:    format,
			Status:    StatusQueued,
			CreatedAt: now,
			UpdatedAt: now,
		},
		done: make(chan struct{}),
	}

	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return Job{}, ErrClosed
	}
	q.gcLocked()
	select {
	case q.tasks <- task{id: id, format: format, body: body, catalog: catalog}:
		q.jobs[id] = st
		snap := st.snapshot()
		q.mu.Unlock()
		q.save(snap, nil, nil)
		q.mu.Lock()
		return snap, nil
	default:
		return Job{}, ErrQueueFull
	}
}

func (q *Queue) Get(id string) (Job, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	st, ok := q.jobs[id]
	if !ok {
		return Job{}, false
	}
	return st.snapshot(), true
}

func (q *Queue) List() []Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]Job, 0, len(q.jobs))
	for _, st := range q.jobs {
		out = append(out, st.snapshot())
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

func (q *Queue) Clear() {
	q.clearFinished(nil)
	q.deleteFinished("")
}

func (q *Queue) ClearKind(kind string) {
	q.clearFinished(func(j Job) bool {
		return JobKind(j.Format) == kind
	})
	q.deleteFinished(kind)
}

func JobKind(format Format) string {
	switch format {
	case FormatXSD, FormatJSONTemplate:
		return KindSchema
	default:
		return KindDocument
	}
}

func (q *Queue) clearFinished(match func(Job) bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for id, st := range q.jobs {
		if st.Status != StatusDone && st.Status != StatusFailed {
			continue
		}
		if match != nil && !match(st.snapshot()) {
			continue
		}
		delete(q.jobs, id)
	}
}

func (q *Queue) Wait(id string, timeout time.Duration) (Job, bool) {
	q.mu.Lock()
	st, ok := q.jobs[id]
	q.mu.Unlock()
	if !ok {
		return Job{}, false
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-st.done:
	case <-timer.C:
	}
	return q.Get(id)
}

func (q *Queue) Close() {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.closed = true
	close(q.tasks)
	q.mu.Unlock()
	q.wg.Wait()
}

func (q *Queue) worker() {
	defer q.wg.Done()
	for t := range q.tasks {
		q.setStatus(t.id, StatusRunning, "", nil)
		var (
			doc Document
			err error
		)
		if t.catalog {
			doc, err = ProcessCatalog(t.format, t.body)
		} else {
			doc, err = q.proc(t.format, t.body)
		}
		if err != nil {
			q.setStatus(t.id, StatusFailed, err.Error(), nil)
			continue
		}
		q.setStatus(t.id, StatusDone, "", doc)
	}
}

func (q *Queue) setStatus(id string, status Status, errMsg string, doc Document) {
	var xmlBody, jsonBody []byte
	if status == StatusDone && doc != nil {
		var encErr error
		xmlBody, encErr = doc.EncodeXML()
		if encErr == nil {
			jsonBody, encErr = doc.EncodeJSON()
		}
		if encErr != nil {
			status = StatusFailed
			errMsg = encErr.Error()
			doc = nil
			xmlBody, jsonBody = nil, nil
		}
	}

	q.mu.Lock()
	st, ok := q.jobs[id]
	if !ok {
		q.mu.Unlock()
		return
	}
	st.Status = status
	st.Error = errMsg
	st.Doc = doc
	st.Catalog = catalogDoc(doc)
	st.XML = xmlBody
	st.JSON = jsonBody
	st.UpdatedAt = time.Now()
	snap := st.snapshot()
	q.mu.Unlock()

	q.save(snap, xmlBody, jsonBody)

	if status == StatusDone && q.onDone != nil && snap.Doc != nil {
		q.onDone(&snap)
	}

	if status == StatusDone || status == StatusFailed {
		q.mu.Lock()
		select {
		case <-st.done:
		default:
			close(st.done)
		}
		q.mu.Unlock()
	}
}

func (q *Queue) save(job Job, xmlBody, jsonBody []byte) {
	if q.persist == nil {
		return
	}
	_ = q.persist.Save(job, xmlBody, jsonBody)
}

func (q *Queue) deleteFinished(kind string) {
	if q.persist == nil {
		return
	}
	_ = q.persist.DeleteFinished(kind)
}

func (q *Queue) gcLocked() {
	var removed []string
	for len(q.jobs) >= q.maxJobs {
		var oldestID string
		var oldest time.Time
		for id, st := range q.jobs {
			if st.Status != StatusDone && st.Status != StatusFailed {
				continue
			}
			if oldestID == "" || st.UpdatedAt.Before(oldest) {
				oldestID = id
				oldest = st.UpdatedAt
			}
		}
		if oldestID == "" {
			break
		}
		delete(q.jobs, oldestID)
		removed = append(removed, oldestID)
	}
	if q.persist != nil {
		for _, id := range removed {
			_ = q.persist.Delete(id)
		}
	}
}

func (j storedJob) snapshot() Job {
	return j.Job
}

func catalogDoc(doc Document) *model.Catalog {
	cat, _ := doc.(*model.Catalog)
	return cat
}

func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

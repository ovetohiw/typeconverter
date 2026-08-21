package api

import (
	"bytes"
	"embed"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"

	"typeconverter/config"
	"typeconverter/jsonconv"
	"typeconverter/queue"
	"typeconverter/schema"
	"typeconverter/store"
)

//go:embed web/*
var webFS embed.FS

const defaultMaxBodyBytes = 10 << 20

type jobTicket struct {
	XMLName xml.Name `xml:"job" json:"-"`
	ID      string   `xml:"id" json:"id"`
	Status  string   `xml:"status" json:"status"`
}

type jobStatus struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Format    string `json:"format"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type schemaDoc struct {
	sch *schema.Schema
	xsd []byte
}

func (d *schemaDoc) EncodeXML() ([]byte, error) {
	if d == nil {
		return schema.EncodeXSD(nil)
	}
	if len(d.xsd) > 0 {
		return d.xsd, nil
	}
	return schema.EncodeXSD(d.sch)
}

func (d *schemaDoc) EncodeJSON() ([]byte, error) {
	if d == nil {
		return schema.EncodeTemplate(nil)
	}
	return schema.EncodeTemplate(d.sch)
}

// Server exposes XML and JSON send/receive HTTP APIs over a catalog or a loaded schema.
type Server struct {
	q            *queue.Queue
	st           *store.Store
	maxBodyBytes int64
	mu           sync.RWMutex
	doc          queue.Document
	sch          *schema.Schema
	xsdRaw       []byte
}

func NewServer() *Server {
	cfg := config.Default()
	cfg.Store.Path = ""
	return NewFromConfig(cfg)
}

func NewFromConfig(cfg config.Config) *Server {
	if err := cfg.Validate(); err != nil {
		cfg = config.Default()
		cfg.Store.Path = ""
	}
	st, err := store.Open(cfg.Store.Path, cfg.Queue.MaxJobs)
	if err != nil {
		st, err = store.Open("", cfg.Queue.MaxJobs)
		if err != nil {
			panic(err)
		}
	}
	s := &Server{maxBodyBytes: cfg.HTTP.MaxBodyBytes, st: st}
	if s.maxBodyBytes <= 0 {
		s.maxBodyBytes = defaultMaxBodyBytes
	}
	qcfg := cfg.QueueConfig()
	qcfg.Persist = sqlitePersist{st: st}
	s.q = queue.New(qcfg, s.process, s.onJobDone)
	s.restore()
	return s
}

func (s *Server) process(format queue.Format, body []byte) (queue.Document, error) {
	switch format {
	case queue.FormatXSD:
		sch, err := schema.ParseXSD(body)
		if err != nil {
			return nil, err
		}
		return &schemaDoc{sch: sch, xsd: append([]byte(nil), body...)}, nil
	case queue.FormatJSONTemplate:
		sch, err := schema.ParseTemplate(body)
		if err != nil {
			return nil, err
		}
		return &schemaDoc{sch: sch}, nil
	}
	s.mu.RLock()
	sch := s.sch
	s.mu.RUnlock()
	if sch != nil {
		return schema.Decode(sch, string(format), body)
	}
	return queue.ProcessCatalog(format, body)
}

func (s *Server) submitBody(r *http.Request, format queue.Format, body []byte) (queue.Job, error) {
	if catalogRequest(r) {
		return s.q.SubmitCatalog(format, body)
	}
	return s.q.Submit(format, body)
}

func catalogRequest(r *http.Request) bool {
	v := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("catalog")))
	return v == "1" || v == "true"
}

func NewServerWith(qcfg queue.Config) *Server {
	cfg := config.Default()
	cfg.Store.Path = ""
	if qcfg.Workers > 0 {
		cfg.Queue.Workers = qcfg.Workers
	}
	if qcfg.QueueSize > 0 {
		cfg.Queue.QueueSize = qcfg.QueueSize
	}
	if qcfg.MaxJobs > 0 {
		cfg.Queue.MaxJobs = qcfg.MaxJobs
	}
	return NewFromConfig(cfg)
}

func NewHandler() http.Handler {
	return NewServer().Handler()
}

func (s *Server) Close() {
	if s.q != nil {
		s.q.Close()
	}
	if s.st != nil {
		_ = s.st.Close()
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", serveWeb("index.html", "text/html; charset=utf-8"))
	mux.HandleFunc("GET /app.css", serveWeb("app.css", "text/css; charset=utf-8"))
	mux.HandleFunc("GET /app.js", serveWeb("app.js", "text/javascript; charset=utf-8"))
	mux.HandleFunc("POST /xml", s.postXML)
	mux.HandleFunc("GET /xml", s.getXML)
	mux.HandleFunc("POST /json", s.postJSON)
	mux.HandleFunc("GET /json", s.getJSON)
	mux.HandleFunc("POST /xsd", s.postXSD)
	mux.HandleFunc("GET /xsd", s.getXSD)
	mux.HandleFunc("POST /jsontemplate", s.postTemplate)
	mux.HandleFunc("GET /jsontemplate", s.getTemplate)
	mux.HandleFunc("GET /schema", s.getSchemaInfo)
	mux.HandleFunc("DELETE /schema", s.deleteSchema)
	mux.HandleFunc("GET /jobs", s.listJobs)
	mux.HandleFunc("DELETE /jobs", s.deleteJobs)
	mux.HandleFunc("GET /jobs/{id}", s.getJob)
	mux.HandleFunc("GET /jobs/{id}/xml", s.getJobXML)
	mux.HandleFunc("GET /jobs/{id}/json", s.getJobJSON)
	return mux
}

func (s *Server) postXML(w http.ResponseWriter, r *http.Request) {
	if !isXMLContentType(r.Header.Get("Content-Type")) {
		writeXMLError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/xml or text/xml")
		return
	}
	body, err := s.readBody(r)
	if err != nil {
		writeXMLError(w, statusForReadError(err), err.Error())
		return
	}
	job, err := s.submitBody(r, queue.FormatXML, body)
	if err != nil {
		writeXMLError(w, statusForQueueError(err), err.Error())
		return
	}
	s.writeJobTicketXML(w, job)
}

func (s *Server) getXML(w http.ResponseWriter, r *http.Request) {
	doc := s.load()
	if doc == nil {
		writeXMLError(w, http.StatusNotFound, "no document")
		return
	}
	s.writeDocXML(w, doc)
}

func (s *Server) postJSON(w http.ResponseWriter, r *http.Request) {
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		writeJSONError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}
	body, err := s.readBody(r)
	if err != nil {
		writeJSONError(w, statusForReadError(err), err.Error())
		return
	}
	job, err := s.submitBody(r, queue.FormatJSON, body)
	if err != nil {
		writeJSONError(w, statusForQueueError(err), err.Error())
		return
	}
	s.writeJobTicketJSON(w, job)
}

func (s *Server) getJSON(w http.ResponseWriter, r *http.Request) {
	doc := s.load()
	if doc == nil {
		writeJSONError(w, http.StatusNotFound, "no document")
		return
	}
	s.writeDocJSON(w, doc)
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	out := s.listJobStatuses()
	body, err := json.Marshal(out)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) deleteJobs(w http.ResponseWriter, r *http.Request) {
	kind := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("kind")))
	switch kind {
	case queue.KindSchema, queue.KindDocument:
		s.q.ClearKind(kind)
	default:
		s.q.Clear()
	}
	writeJSON(w, http.StatusOK, []byte(`{"status":"cleared"}`))
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	job, ok := s.lookupJob(r.PathValue("id"))
	if !ok {
		writeJSONError(w, http.StatusNotFound, "job not found")
		return
	}
	body, err := json.Marshal(toJobStatus(job))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) getJobXML(w http.ResponseWriter, r *http.Request) {
	job, ok := s.readyJob(w, r.PathValue("id"), true)
	if !ok {
		return
	}
	s.writeDocXML(w, job.Doc)
}

func (s *Server) getJobJSON(w http.ResponseWriter, r *http.Request) {
	job, ok := s.readyJob(w, r.PathValue("id"), false)
	if !ok {
		return
	}
	s.writeDocJSON(w, job.Doc)
}

func (s *Server) readyJob(w http.ResponseWriter, id string, asXML bool) (queue.Job, bool) {
	job, ok := s.lookupJob(id)
	if !ok {
		writeJobError(w, asXML, http.StatusNotFound, "job not found")
		return queue.Job{}, false
	}
	switch job.Status {
	case queue.StatusDone:
		if job.Doc == nil {
			writeJobError(w, asXML, http.StatusInternalServerError, "job has no document")
			return queue.Job{}, false
		}
		return job, true
	case queue.StatusFailed:
		writeJobError(w, asXML, http.StatusUnprocessableEntity, job.Error)
		return queue.Job{}, false
	default:
		writeJobError(w, asXML, http.StatusConflict, "job not ready")
		return queue.Job{}, false
	}
}

func (s *Server) lookupJob(id string) (queue.Job, bool) {
	if job, ok := s.q.Get(id); ok {
		return job, true
	}
	if s.st == nil {
		return queue.Job{}, false
	}
	rec, ok, err := s.st.Get(id)
	if err != nil || !ok {
		return queue.Job{}, false
	}
	return jobFromRecord(rec), true
}

func (s *Server) listJobStatuses() []jobStatus {
	if s.st != nil {
		recs, err := s.st.List()
		if err == nil {
			out := make([]jobStatus, 0, len(recs))
			for _, rec := range recs {
				out = append(out, recStatus(rec))
			}
			return out
		}
	}
	jobs := s.q.List()
	out := make([]jobStatus, 0, len(jobs))
	for _, job := range jobs {
		out = append(out, toJobStatus(job))
	}
	return out
}

func (s *Server) writeJobTicketXML(w http.ResponseWriter, job queue.Job) {
	ticket := jobTicket{ID: job.ID, Status: string(job.Status)}
	body, err := xml.Marshal(ticket)
	if err != nil {
		writeXMLError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Location", "/jobs/"+job.ID)
	writeXML(w, http.StatusAccepted, append([]byte(xml.Header), body...))
}

func (s *Server) writeJobTicketJSON(w http.ResponseWriter, job queue.Job) {
	body, err := jsonconv.Encode(jobTicket{ID: job.ID, Status: string(job.Status)})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Location", "/jobs/"+job.ID)
	writeJSON(w, http.StatusAccepted, body)
}

func (s *Server) writeDocXML(w http.ResponseWriter, doc queue.Document) {
	encoded, err := doc.EncodeXML()
	if err != nil {
		writeXMLError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeXML(w, http.StatusOK, encoded)
}

func (s *Server) writeDocJSON(w http.ResponseWriter, doc queue.Document) {
	encoded, err := doc.EncodeJSON()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, encoded)
}

func (s *Server) onJobDone(job *queue.Job) {
	if job == nil || job.Doc == nil {
		return
	}
	if doc, ok := job.Doc.(*schemaDoc); ok {
		s.setSchema(doc.sch, doc.xsd)
		return
	}
	s.store(job.Doc)
}

func (s *Server) store(doc queue.Document) {
	s.mu.Lock()
	s.doc = doc
	s.mu.Unlock()
	s.persistDocument(doc)
}

func (s *Server) persistDocument(doc queue.Document) {
	if s.st == nil || doc == nil {
		return
	}
	xmlBody, err := doc.EncodeXML()
	if err != nil {
		return
	}
	jsonBody, err := doc.EncodeJSON()
	if err != nil {
		return
	}
	_ = s.st.SetDocument(xmlBody, jsonBody)
}

func (s *Server) load() queue.Document {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.doc
}

func (s *Server) restore() {
	if s.st == nil {
		return
	}
	if xmlBody, jsonBody, ok, err := s.st.Document(); err == nil && ok {
		s.doc = &blobDoc{xml: xmlBody, json: jsonBody}
	}
	root, xsd, tpl, ok, err := s.st.Schema()
	if err != nil || !ok {
		return
	}
	var sch *schema.Schema
	if len(xsd) > 0 {
		sch, err = schema.ParseXSD(xsd)
	}
	if (sch == nil || err != nil) && len(tpl) > 0 {
		sch, err = schema.ParseTemplate(tpl)
	}
	if err != nil || sch == nil {
		return
	}
	if sch.Root == "" {
		sch.Root = root
	}
	s.sch = sch
	s.xsdRaw = xsd
}

func (s *Server) postXSD(w http.ResponseWriter, r *http.Request) {
	if !isXMLContentType(r.Header.Get("Content-Type")) && !isXSDContentType(r.Header.Get("Content-Type")) {
		writeXMLError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/xml, text/xml, or application/xsd+xml")
		return
	}
	body, err := s.readBody(r)
	if err != nil {
		writeXMLError(w, statusForReadError(err), err.Error())
		return
	}
	job, err := s.q.Submit(queue.FormatXSD, body)
	if err != nil {
		writeXMLError(w, statusForQueueError(err), err.Error())
		return
	}
	s.writeJobTicketXML(w, job)
}

func (s *Server) getXSD(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	sch := s.sch
	raw := s.xsdRaw
	s.mu.RUnlock()
	if sch == nil {
		writeXMLError(w, http.StatusNotFound, "no schema")
		return
	}
	if len(raw) > 0 {
		writeXML(w, http.StatusOK, raw)
		return
	}
	encoded, err := schema.EncodeXSD(sch)
	if err != nil {
		writeXMLError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeXML(w, http.StatusOK, encoded)
}

func (s *Server) postTemplate(w http.ResponseWriter, r *http.Request) {
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		writeJSONError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}
	body, err := s.readBody(r)
	if err != nil {
		writeJSONError(w, statusForReadError(err), err.Error())
		return
	}
	job, err := s.q.Submit(queue.FormatJSONTemplate, body)
	if err != nil {
		writeJSONError(w, statusForQueueError(err), err.Error())
		return
	}
	s.writeJobTicketJSON(w, job)
}

func (s *Server) getTemplate(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	sch := s.sch
	s.mu.RUnlock()
	if sch == nil {
		writeJSONError(w, http.StatusNotFound, "no schema")
		return
	}
	s.writeTemplate(w)
}

func (s *Server) getSchemaInfo(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	sch := s.sch
	s.mu.RUnlock()
	if sch == nil {
		writeJSONError(w, http.StatusNotFound, "no schema")
		return
	}
	body, err := jsonconv.Encode(map[string]string{"status": "loaded", "root": sch.Root})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) deleteSchema(w http.ResponseWriter, r *http.Request) {
	s.setSchema(nil, nil)
	writeJSON(w, http.StatusOK, []byte(`{"status":"cleared"}`))
}

func (s *Server) setSchema(sch *schema.Schema, xsd []byte) {
	s.mu.Lock()
	s.sch = sch
	s.xsdRaw = xsd
	s.doc = nil
	s.mu.Unlock()
	if s.st == nil {
		return
	}
	_ = s.st.ClearDocument()
	if sch == nil {
		_ = s.st.ClearSchema()
		return
	}
	if len(xsd) == 0 {
		encoded, err := schema.EncodeXSD(sch)
		if err == nil {
			xsd = encoded
		}
	}
	tpl, err := schema.EncodeTemplate(sch)
	if err != nil {
		return
	}
	_ = s.st.SetSchema(sch.Root, xsd, tpl)
}

func (s *Server) writeTemplate(w http.ResponseWriter) {
	s.mu.RLock()
	sch := s.sch
	s.mu.RUnlock()
	body, err := schema.EncodeTemplate(sch)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) readBody(r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, s.maxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > s.maxBodyBytes {
		return nil, errBodyTooLarge
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, errEmptyBody
	}
	return body, nil
}

type apiError string

func (e apiError) Error() string { return string(e) }

const (
	errBodyTooLarge apiError = "body too large"
	errEmptyBody    apiError = "body is empty"
)

func statusForReadError(err error) int {
	switch err {
	case errBodyTooLarge:
		return http.StatusRequestEntityTooLarge
	case errEmptyBody:
		return http.StatusBadRequest
	default:
		return http.StatusBadRequest
	}
}

func statusForQueueError(err error) int {
	switch {
	case errors.Is(err, queue.ErrQueueFull), errors.Is(err, queue.ErrClosed):
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

func isXMLContentType(ct string) bool {
	if ct == "" {
		return true
	}
	switch mediaType(ct) {
	case "application/xml", "text/xml":
		return true
	default:
		return false
	}
}

func isXSDContentType(ct string) bool {
	if ct == "" {
		return true
	}
	switch mediaType(ct) {
	case "application/xsd+xml", "application/xml", "text/xml":
		return true
	default:
		return false
	}
}

func isJSONContentType(ct string) bool {
	if ct == "" {
		return true
	}
	return mediaType(ct) == "application/json"
}

func mediaType(ct string) string {
	media, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
	}
	return media
}

func toJobStatus(job queue.Job) jobStatus {
	return jobStatus{
		ID:        job.ID,
		Kind:      queue.JobKind(job.Format),
		Format:    string(job.Format),
		Status:    string(job.Status),
		Error:     job.Error,
		CreatedAt: job.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt: job.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func serveWeb(name, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := webFS.ReadFile("web/" + name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}
}

func writeXML(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func writeJSON(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func writeXMLError(w http.ResponseWriter, status int, msg string) {
	var b strings.Builder
	b.WriteString("<error>")
	_ = xml.EscapeText(&b, []byte(msg))
	b.WriteString("</error>")
	writeXML(w, status, []byte(b.String()))
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	encoded, err := jsonconv.Encode(map[string]string{"error": msg})
	if err != nil {
		http.Error(w, msg, status)
		return
	}
	writeJSON(w, status, encoded)
}

func writeJobError(w http.ResponseWriter, asXML bool, status int, msg string) {
	if asXML {
		writeXMLError(w, status, msg)
		return
	}
	writeJSONError(w, status, msg)
}

package api

import (
	"typeconverter/queue"
	"typeconverter/store"
)

type blobDoc struct {
	xml  []byte
	json []byte
}

func (d *blobDoc) EncodeXML() ([]byte, error)  { return d.xml, nil }
func (d *blobDoc) EncodeJSON() ([]byte, error) { return d.json, nil }

type sqlitePersist struct {
	st *store.Store
}

func (p sqlitePersist) Save(job queue.Job, xmlBody, jsonBody []byte) error {
	if p.st == nil {
		return nil
	}
	return p.st.Put(store.Record{
		ID:        job.ID,
		Kind:      queue.JobKind(job.Format),
		Format:    string(job.Format),
		Status:    string(job.Status),
		Error:     job.Error,
		XML:       xmlBody,
		JSON:      jsonBody,
		CreatedAt: job.CreatedAt,
		UpdatedAt: job.UpdatedAt,
	})
}

func (p sqlitePersist) Delete(id string) error {
	if p.st == nil {
		return nil
	}
	return p.st.Delete(id)
}

func (p sqlitePersist) DeleteFinished(kind string) error {
	if p.st == nil {
		return nil
	}
	return p.st.DeleteFinished(kind)
}

func jobFromRecord(rec store.Record) queue.Job {
	return queue.Job{
		ID:        rec.ID,
		Format:    queue.Format(rec.Format),
		Status:    queue.Status(rec.Status),
		Error:     rec.Error,
		Doc:       &blobDoc{xml: rec.XML, json: rec.JSON},
		XML:       rec.XML,
		JSON:      rec.JSON,
		CreatedAt: rec.CreatedAt,
		UpdatedAt: rec.UpdatedAt,
	}
}

func recStatus(rec store.Record) jobStatus {
	return toJobStatus(jobFromRecord(rec))
}

package upload

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"mellium.im/xmpp/jid"
)

// Slot represents an HTTP upload slot
type Slot struct {
	GetURL  string
	PutURL  string
	Headers map[string]string
}

// Upload represents an ongoing or completed upload
type Upload struct {
	ID       string
	Filename string
	Size     int64
	MIMEType string
	Slot     *Slot
	Progress float64
	Done     bool
	Error    error
	URL      string
}

// Manager manages HTTP file uploads
type Manager struct {
	mu        sync.RWMutex
	uploads   map[string]*Upload
	uploadJID jid.JID
	maxSize   int64
	client    *http.Client
}

// NewManager creates a new upload manager
func NewManager() *Manager {
	return &Manager{
		uploads: make(map[string]*Upload),
		maxSize: 10 * 1024 * 1024, // 10MB default
		client:  &http.Client{},
	}
}

// SetUploadService sets the JID of the HTTP upload service
func (m *Manager) SetUploadService(j jid.JID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.uploadJID = j
}

// SetMaxSize sets the maximum upload size
func (m *Manager) SetMaxSize(size int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maxSize = size
}

// GetUploadService returns the upload service JID
func (m *Manager) GetUploadService() jid.JID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.uploadJID
}

// UploadFile uploads a file
func (m *Manager) UploadFile(ctx context.Context, id, path string, slot *Slot) (*Upload, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	mimeType := mime.TypeByExtension(filepath.Ext(path))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	upload := &Upload{
		ID:       id,
		Filename: filepath.Base(path),
		Size:     stat.Size(),
		MIMEType: mimeType,
		Slot:     slot,
	}

	m.mu.Lock()
	m.uploads[id] = upload
	m.mu.Unlock()

	go m.performUpload(ctx, upload, file)

	return upload, nil
}

// UploadData uploads raw data
func (m *Manager) UploadData(ctx context.Context, id, filename string, data []byte, slot *Slot) (*Upload, error) {
	mimeType := mime.TypeByExtension(filepath.Ext(filename))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	upload := &Upload{
		ID:       id,
		Filename: filename,
		Size:     int64(len(data)),
		MIMEType: mimeType,
		Slot:     slot,
	}

	m.mu.Lock()
	m.uploads[id] = upload
	m.mu.Unlock()

	go m.performUpload(ctx, upload, bytes.NewReader(data))

	return upload, nil
}

// performUpload performs the actual HTTP upload
func (m *Manager) performUpload(ctx context.Context, upload *Upload, reader io.Reader) {
	req, err := http.NewRequestWithContext(ctx, "PUT", upload.Slot.PutURL, reader)
	if err != nil {
		m.setUploadError(upload.ID, err)
		return
	}

	req.Header.Set("Content-Type", upload.MIMEType)
	req.ContentLength = upload.Size

	for k, v := range upload.Slot.Headers {
		req.Header.Set(k, v)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		m.setUploadError(upload.ID, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		m.setUploadError(upload.ID, fmt.Errorf("upload failed with status: %d", resp.StatusCode))
		return
	}

	m.mu.Lock()
	if u, ok := m.uploads[upload.ID]; ok {
		u.Done = true
		u.Progress = 100
		u.URL = upload.Slot.GetURL
	}
	m.mu.Unlock()
}

// setUploadError sets an error on an upload
func (m *Manager) setUploadError(id string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if u, ok := m.uploads[id]; ok {
		u.Error = err
		u.Done = true
	}
}

// GetUpload returns an upload by ID
func (m *Manager) GetUpload(id string) *Upload {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.uploads[id]
}

// RemoveUpload removes an upload from tracking
func (m *Manager) RemoveUpload(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.uploads, id)
}

// GetAllUploads returns all uploads
func (m *Manager) GetAllUploads() []*Upload {
	m.mu.RLock()
	defer m.mu.RUnlock()

	uploads := make([]*Upload, 0, len(m.uploads))
	for _, upload := range m.uploads {
		uploads = append(uploads, upload)
	}
	return uploads
}

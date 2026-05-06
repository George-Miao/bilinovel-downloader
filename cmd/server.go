package cmd

import (
	"bilinovel-downloader/utils"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

const (
	envDownloadDir   = "DOWNLOAD_DIR"
	envAuxDir        = "AUX_DIR"
	envCleanAuxFiles = "CLEAN_AUX_FILES"
	envServerAddr    = "SERVER_ADDR"

	jobQueueSize = 128
)

type serverConfig struct {
	Addr          string
	DownloadDir   string
	AuxDir        string
	CleanAuxFiles bool
}

type serverResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	JobID   string `json:"job_id,omitempty"`
}

type jobStatus string

const (
	jobStatusQueued    jobStatus = "queued"
	jobStatusRunning   jobStatus = "running"
	jobStatusSucceeded jobStatus = "succeeded"
	jobStatusFailed    jobStatus = "failed"
	jobStatusCanceling jobStatus = "canceling"
	jobStatusCanceled  jobStatus = "canceled"
)

type downloadJob struct {
	ID        string    `json:"id"`
	Status    jobStatus `json:"status"`
	NovelID   int       `json:"novel_id"`
	VolumeID  int       `json:"volume_id,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	StartedAt time.Time `json:"started_at,omitempty"`
	EndedAt   *time.Time `json:"ended_at"`

	cancel context.CancelFunc
}

type jobManager struct {
	config serverConfig
	queue  chan *downloadJob

	mu   sync.RWMutex
	jobs map[string]*downloadJob
}

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the long running download command server",
	Long:  "Start the long running download command server",
	RunE:  runServer,
}

func init() {
	RootCmd.AddCommand(serverCmd)
}

func runServer(cmd *cobra.Command, args []string) error {
	config, err := loadServerConfigFromEnv()
	if err != nil {
		return err
	}

	manager := newJobManager(config)
	server := newDownloadServer(config, manager)
	slog.Info("starting command server", slog.String("addr", config.Addr), slog.String("downloadDir", config.DownloadDir))
	return http.ListenAndServe(config.Addr, server)
}

func loadServerConfigFromEnv() (serverConfig, error) {
	config := serverConfig{
		Addr:        os.Getenv(envServerAddr),
		DownloadDir: os.Getenv(envDownloadDir),
		AuxDir:      os.Getenv(envAuxDir),
	}
	if config.Addr == "" {
		config.Addr = ":8080"
	}
	if config.DownloadDir == "" {
		config.DownloadDir = "novels"
	}
	if config.AuxDir == "" {
		config.AuxDir = config.DownloadDir
	}
	cleanAuxFiles, err := strconv.ParseBool(os.Getenv(envCleanAuxFiles))
	if err != nil && os.Getenv(envCleanAuxFiles) != "" {
		return serverConfig{}, fmt.Errorf("%s must be a boolean", envCleanAuxFiles)
	}
	config.CleanAuxFiles = cleanAuxFiles
	return config, nil
}

func newJobManager(config serverConfig) *jobManager {
	manager := &jobManager{
		config: config,
		queue:  make(chan *downloadJob, jobQueueSize),
		jobs:   make(map[string]*downloadJob),
	}
	go manager.run()
	return manager
}

func (m *jobManager) createJob(novelID int, volumeID int) (*downloadJob, bool) {
	job := &downloadJob{
		ID:        uuid.NewString(),
		Status:    jobStatusQueued,
		NovelID:   novelID,
		VolumeID:  volumeID,
		CreatedAt: time.Now().UTC(),
		cancel:    func() {},
	}

	m.mu.Lock()
	m.jobs[job.ID] = job
	m.mu.Unlock()

	select {
	case m.queue <- job:
		return m.snapshotJob(job), true
	default:
		m.mu.Lock()
		job.Status = jobStatusFailed
		job.Error = "job queue is full"
		t := time.Now().UTC()
		job.EndedAt = &t
		m.mu.Unlock()
		return m.snapshotJob(job), false
	}
}

func (m *jobManager) getJob(jobID string) (*downloadJob, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	job, ok := m.jobs[jobID]
	if !ok {
		return nil, false
	}
	return cloneJob(job), true
}

func (m *jobManager) listJobs() []*downloadJob {
	m.mu.RLock()
	defer m.mu.RUnlock()

	jobs := make([]*downloadJob, 0, len(m.jobs))
	for _, job := range m.jobs {
		jobs = append(jobs, cloneJob(job))
	}
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].CreatedAt.After(jobs[j].CreatedAt)
	})
	return jobs
}

func (m *jobManager) cancelJob(jobID string) (*downloadJob, bool, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	job, ok := m.jobs[jobID]
	if !ok {
		return nil, false, false
	}
	cancellationRequested := false
	switch job.Status {
	case jobStatusQueued:
		job.Status = jobStatusCanceled
		t := time.Now().UTC()
		job.EndedAt = &t
		job.cancel()
		cancellationRequested = true
	case jobStatusRunning:
		job.Status = jobStatusCanceling
		job.cancel()
		cancellationRequested = true
	}
	return cloneJob(job), true, cancellationRequested
}

func (m *jobManager) run() {
	for job := range m.queue {
		m.runJob(job)
	}
}

func (m *jobManager) runJob(job *downloadJob) {
	ctx, cancel := context.WithCancel(context.Background())

	m.mu.Lock()
	if job.Status == jobStatusCanceled {
		m.mu.Unlock()
		cancel()
		return
	}
	job.Status = jobStatusRunning
	job.StartedAt = time.Now().UTC()
	job.cancel = cancel
	m.mu.Unlock()

	err := runDownload(downloadOptions{
		Context:              ctx,
		NovelId:              job.NovelID,
		VolumeId:             job.VolumeID,
		OutputPath:           m.config.DownloadDir,
		AuxPath:              m.config.AuxDir,
		OutputType:           "epub",
		Concurrency:          3,
		OrganizeByNovelTitle: true,
		CleanAux:             m.config.CleanAuxFiles,
	})

	m.mu.Lock()
	defer m.mu.Unlock()

	endedAt := time.Now().UTC()
	job.EndedAt = &endedAt
	switch {
	case ctx.Err() != nil || job.Status == jobStatusCanceling:
		job.Status = jobStatusCanceled
		if err != nil {
			job.Error = err.Error()
		}
	case err != nil:
		job.Status = jobStatusFailed
		job.Error = err.Error()
	default:
		job.Status = jobStatusSucceeded
	}
}

func (m *jobManager) snapshotJob(job *downloadJob) *downloadJob {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneJob(job)
}

func cloneJob(job *downloadJob) *downloadJob {
	return &downloadJob{
		ID:        job.ID,
		Status:    job.Status,
		NovelID:   job.NovelID,
		VolumeID:  job.VolumeID,
		Error:     job.Error,
		CreatedAt: job.CreatedAt,
		StartedAt: job.StartedAt,
		EndedAt:   job.EndedAt,
	}
}

func newDownloadServer(config serverConfig, manager *jobManager) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/download/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET, POST")
			writeServerError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		novelID, volumeID, err := parseDownloadPath(r.URL.Path)
		if err != nil {
			writeServerError(w, http.StatusBadRequest, err.Error())
			return
		}

		exists, err := novelExists(r.Context(), novelID)
		if err != nil {
			writeServerError(w, http.StatusBadGateway, err.Error())
			return
		}
		if !exists {
			writeServerError(w, http.StatusBadRequest, "novel not found")
			return
		}

		job, ok := manager.createJob(novelID, volumeID)
		if !ok {
			writeServerError(w, http.StatusServiceUnavailable, job.Error)
			return
		}

		writeServerJSON(w, http.StatusAccepted, serverResponse{
			Status: "queued",
			JobID:  job.ID,
		})
	})

	mux.HandleFunc("/job", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			writeServerError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		writeServerJSON(w, http.StatusOK, manager.listJobs())
	})

	mux.HandleFunc("/job/", func(w http.ResponseWriter, r *http.Request) {
		jobID, err := parseJobPath(r.URL.Path)
		if err != nil {
			writeServerError(w, http.StatusBadRequest, err.Error())
			return
		}

		switch r.Method {
		case http.MethodGet:
			job, ok := manager.getJob(jobID)
			if !ok {
				writeServerError(w, http.StatusNotFound, "job not found")
				return
			}
			writeServerJSON(w, http.StatusOK, job)
		case http.MethodDelete:
			job, ok, cancellationRequested := manager.cancelJob(jobID)
			if !ok {
				writeServerError(w, http.StatusNotFound, "job not found")
				return
			}
			if cancellationRequested {
				writeServerJSON(w, http.StatusAccepted, job)
				return
			}
			writeServerJSON(w, http.StatusOK, job)
		default:
			w.Header().Set("Allow", "GET, DELETE")
			writeServerError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	return mux
}

func parseDownloadPath(path string) (int, int, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 && len(parts) != 3 {
		return 0, 0, fmt.Errorf("expected /download/{novel_id} or /download/{novel_id}/{vol_id}")
	}
	if parts[0] != "download" {
		return 0, 0, fmt.Errorf("expected download endpoint")
	}

	novelID, err := strconv.Atoi(parts[1])
	if err != nil || novelID <= 0 {
		return 0, 0, fmt.Errorf("novel_id must be a positive integer")
	}

	if len(parts) == 2 {
		return novelID, 0, nil
	}

	volumeID, err := strconv.Atoi(parts[2])
	if err != nil || volumeID <= 0 {
		return 0, 0, fmt.Errorf("vol_id must be a positive integer")
	}
	return novelID, volumeID, nil
}

func parseJobPath(path string) (string, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || parts[0] != "job" || parts[1] == "" {
		return "", fmt.Errorf("expected /job/{job_id}")
	}
	return parts[1], nil
}

func novelExists(ctx context.Context, novelID int) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := utils.NewRestyClient(1).R().
		SetContext(ctx).
		Get(fmt.Sprintf("https://www.bilinovel.com/novel/%d.html", novelID))
	if err != nil {
		return false, fmt.Errorf("failed to validate novel: %w", err)
	}

	switch resp.StatusCode() {
	case http.StatusOK:
		doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body()))
		if err != nil {
			return false, fmt.Errorf("failed to parse novel validation response: %w", err)
		}
		return strings.TrimSpace(doc.Find(".book-title").First().Text()) != "", nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("failed to validate novel: %s", resp.Status())
	}
}

func writeServerError(w http.ResponseWriter, status int, message string) {
	writeServerJSON(w, status, serverResponse{
		Status:  "error",
		Message: message,
	})
}

func writeServerJSON(w http.ResponseWriter, status int, response any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}

package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/example/task-scheduler/scheduler"
)

// Server exposes the scheduler over HTTP
type Server struct {
	sched  *scheduler.Scheduler
	log    *slog.Logger
	mux    *http.ServeMux
	server *http.Server
}

// NewServer creates a new API server
func NewServer(addr string, sched *scheduler.Scheduler, log *slog.Logger) *Server {
	s := &Server{
		sched: sched,
		log:   log,
		mux:   http.NewServeMux(),
	}
	s.routes()
	s.server = &http.Server{
		Addr:         addr,
		Handler:      s.middlewareChain(s.mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("POST /api/tasks", s.handleSubmitTask)
	s.mux.HandleFunc("GET /api/tasks", s.handleListTasks)
	s.mux.HandleFunc("GET /api/tasks/{id}", s.handleGetTask)
	s.mux.HandleFunc("DELETE /api/tasks/{id}", s.handleCancelTask)
	s.mux.HandleFunc("GET /api/stats", s.handleStats)
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
}

// Start begins serving HTTP requests
func (s *Server) Start() error {
	s.log.Info("API server starting", "addr", s.server.Addr)
	return s.server.ListenAndServe()
}

// Stop shuts down the server
func (s *Server) Stop() error {
	return s.server.Close()
}

// SubmitTaskRequest is the JSON payload for submitting a task
type SubmitTaskRequest struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	Payload     map[string]any    `json:"payload"`
	Priority    int               `json:"priority"`
	ScheduledAt *time.Time        `json:"scheduled_at,omitempty"`
	CronExpr    string            `json:"cron_expr,omitempty"`
	MaxRetries  int               `json:"max_retries"`
	RetryDelay  string            `json:"retry_delay,omitempty"` // e.g. "5s", "1m"
	Timeout     string            `json:"timeout,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	DependsOn   []string          `json:"depends_on,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

func (s *Server) handleSubmitTask(w http.ResponseWriter, r *http.Request) {
	var req SubmitTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.Name == "" {
		s.writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Type == "" {
		s.writeError(w, http.StatusBadRequest, "type is required")
		return
	}

	// Build options
	var opts []scheduler.TaskOption

	if req.Priority > 0 {
		opts = append(opts, scheduler.WithPriority(scheduler.Priority(req.Priority)))
	}
	if req.ScheduledAt != nil {
		opts = append(opts, scheduler.WithSchedule(*req.ScheduledAt))
	}
	if req.CronExpr != "" {
		opts = append(opts, scheduler.WithCron(req.CronExpr))
	}
	if req.MaxRetries > 0 {
		delay := 5 * time.Second
		if req.RetryDelay != "" {
			if d, err := time.ParseDuration(req.RetryDelay); err == nil {
				delay = d
			}
		}
		opts = append(opts, scheduler.WithRetry(req.MaxRetries, delay, 2.0))
	}
	if req.Timeout != "" {
		if d, err := time.ParseDuration(req.Timeout); err == nil {
			opts = append(opts, scheduler.WithTimeout(d))
		}
	}
	if len(req.Tags) > 0 {
		opts = append(opts, scheduler.WithTags(req.Tags...))
	}
	if len(req.DependsOn) > 0 {
		opts = append(opts, scheduler.WithDependencies(req.DependsOn...))
	}
	for k, v := range req.Metadata {
		opts = append(opts, scheduler.WithMetadata(k, v))
	}

	task, err := s.sched.Submit(req.Name, req.Type, req.Payload, opts...)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.writeJSON(w, http.StatusCreated, task)
}

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	statusFilter := r.URL.Query().Get("status")

	var tasks []*scheduler.Task
	var err error

	if statusFilter != "" {
		tasks, err = s.sched.ListTasks(scheduler.TaskStatus(statusFilter))
	} else {
		tasks, err = s.sched.ListTasks()
	}

	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"tasks": tasks,
		"count": len(tasks),
	})
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "task ID required")
		return
	}

	task, err := s.sched.GetTask(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, task)
}

func (s *Server) handleCancelTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "task ID required")
		return
	}

	if err := s.sched.Cancel(id); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{
		"message": fmt.Sprintf("task %s cancelled", id),
	})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, s.sched.Stats())
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

// Middleware
func (s *Server) middlewareChain(next http.Handler) http.Handler {
	return s.loggingMiddleware(s.corsMiddleware(s.recoveryMiddleware(next)))
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, statusCode: 200}
		next.ServeHTTP(rw, r)
		s.log.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.statusCode,
			"duration", time.Since(start),
			"remote_addr", r.RemoteAddr,
		)
	})
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("panic recovered", "error", rec, "path", r.URL.Path)
				s.writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Helpers
func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.log.Error("failed to encode response", "error", err)
	}
}

func (s *Server) writeError(w http.ResponseWriter, status int, msg string) {
	s.writeJSON(w, status, map[string]string{
		"error": msg,
	})
}

// RouterInfo returns human-readable route documentation
func RouterInfo() string {
	routes := []string{
		"POST   /api/tasks         Submit a new task",
		"GET    /api/tasks         List all tasks (?status=pending|running|completed|failed)",
		"GET    /api/tasks/{id}    Get task details",
		"DELETE /api/tasks/{id}    Cancel a task",
		"GET    /api/stats         Scheduler statistics",
		"GET    /api/health        Health check",
	}
	return strings.Join(routes, "\n")
}

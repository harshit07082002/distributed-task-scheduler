package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/example/task-scheduler/scheduler"
	"github.com/gin-gonic/gin"
)

// Server exposes the scheduler over HTTP using Gin
type Server struct {
	sched  *scheduler.Scheduler
	log    *slog.Logger
	router *gin.Engine
	server *http.Server
}

// NewServer creates a new API server
func NewServer(addr string, sched *scheduler.Scheduler, log *slog.Logger) *Server {
	gin.SetMode(gin.ReleaseMode)

	s := &Server{
		sched:  sched,
		log:    log,
		router: gin.New(),
	}

	s.router.Use(s.loggingMiddleware(), gin.Recovery())
	s.routes()

	s.server = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	return s
}

func (s *Server) routes() {
	api := s.router.Group("/api")
	{
		api.POST("/tasks", s.handleSubmitTask)
		api.GET("/tasks", s.handleListTasks)
		api.GET("/tasks/:id", s.handleGetTask)
		api.DELETE("/tasks/:id", s.handleCancelTask)
		api.GET("/stats", s.handleStats)
		api.GET("/health", s.handleHealth)
	}
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
	RetryDelay  string            `json:"retry_delay,omitempty"`
	Timeout     string            `json:"timeout,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	DependsOn   []string          `json:"depends_on,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

func (s *Server) loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		s.log.Info("http request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration", time.Since(start),
			"remote_addr", c.ClientIP(),
		)
	}
}

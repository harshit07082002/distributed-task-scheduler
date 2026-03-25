package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/example/task-scheduler/scheduler"
	"github.com/gin-gonic/gin"
)

func (s *Server) handleSubmitTask(c *gin.Context) {
	var req SubmitTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if req.Type == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type is required"})
		return
	}

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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, task)
}

func (s *Server) handleListTasks(c *gin.Context) {
	statusFilter := c.Query("status")

	var tasks []*scheduler.Task
	var err error
	if statusFilter != "" {
		tasks, err = s.sched.ListTasks(scheduler.TaskStatus(statusFilter))
	} else {
		tasks, err = s.sched.ListTasks()
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"tasks": tasks, "count": len(tasks)})
}

func (s *Server) handleGetTask(c *gin.Context) {
	task, err := s.sched.GetTask(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, task)
}

func (s *Server) handleCancelTask(c *gin.Context) {
	id := c.Param("id")
	if err := s.sched.Cancel(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("task %s cancelled", id)})
}

func (s *Server) handleStats(c *gin.Context) {
	c.JSON(http.StatusOK, s.sched.Stats())
}

func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

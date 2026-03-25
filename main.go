package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/example/task-scheduler/api"
	config "github.com/example/task-scheduler/configs"
	"github.com/example/task-scheduler/scheduler"
	"github.com/example/task-scheduler/storage"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	if err := config.Initialize(); err != nil {
		log.Error("failed to initialize config", "error", err)
		os.Exit(1)
	}

	redis := initialiseRedis()

	// ── Scheduler ────────────────────────────────────────────────────────
	cfg := scheduler.Config{
		PollInterval:    time.Duration(config.GetConfig().TaskScheduler.PollInterval) * time.Millisecond,
		ShutdownTimeout: 30 * time.Second,
	}
	sched := scheduler.New(cfg, redis, log)

	// ── Register Task Handlers ───────────────────────────────────────────
	registerHandlers(sched, log)

	// ── Hooks ────────────────────────────────────────────────────────────
	sched.OnTaskComplete(func(task *scheduler.Task, result scheduler.TaskResult) {
		log.Info("✓ task succeeded",
			"task_id", task.ID,
			"task_name", task.Name,
			"duration", result.Duration,
		)
	})

	sched.OnTaskFail(func(task *scheduler.Task, err error) {
		log.Warn("✗ task failed permanently",
			"task_id", task.ID,
			"task_name", task.Name,
			"error", err,
		)
	})

	// ── Start Scheduler ──────────────────────────────────────────────────
	if err := sched.Start(); err != nil {
		log.Error("failed to start scheduler", "error", err)
		os.Exit(1)
	}

	port := config.GetConfig().TaskScheduler.Port
	// ── API Server ───────────────────────────────────────────────────────
	server := api.NewServer(fmt.Sprintf(":%d", port), sched, log)
	go func() {
		log.Info("starting API server", "port", port)
		if err := server.Start(); err != nil {
			log.Info("server stopped", "reason", err)
		}
	}()

	// ── Graceful Shutdown ────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	log.Info("received shutdown signal")
	if err := server.Stop(); err != nil {
		log.Warn("server stop error", "error", err)
	}
	sched.Stop()
}

func initialiseRedis() scheduler.TaskStore {
	host := config.GetConfig().RedisClient.Host
	if host == "" {
		host = "localhost"
	}
	port := config.GetConfig().RedisClient.Port
	if port == "" {
		port = "6379"
	}
	password := config.GetConfig().RedisClient.Password

	store, err := storage.NewRedisStore(host+":"+port, password, 0)
	if err != nil {
		log.Fatalf("failed to connect to redis at %s: %v", host+":"+port, err)
	}
	log.Printf("connected to redis: %s", host+":"+port)
	return store
}

// registerHandlers wires task type strings to handler functions
func registerHandlers(sched *scheduler.Scheduler, log *slog.Logger) {
	// email_notification: simulates sending an email
	sched.Register("email_notification", func(ctx context.Context, task *scheduler.Task) (map[string]any, error) {
		to, _ := task.Payload["to"].(string)
		subject, _ := task.Payload["subject"].(string)

		// Simulate network call
		time.Sleep(time.Duration(50+rand.Intn(100)) * time.Millisecond)

		log.Info("email sent", "to", to, "subject", subject)
		return map[string]any{
			"message_id": fmt.Sprintf("msg_%d", time.Now().UnixMilli()),
			"delivered":  true,
		}, nil
	})

	// data_export: simulates a data export job
	sched.Register("data_export", func(ctx context.Context, task *scheduler.Task) (map[string]any, error) {
		format, _ := task.Payload["format"].(string)
		rows := rand.Intn(10000) + 100

		// Simulate longer processing
		time.Sleep(time.Duration(200+rand.Intn(500)) * time.Millisecond)

		return map[string]any{
			"rows_exported": rows,
			"format":        format,
			"file_size_kb":  rows / 10,
		}, nil
	})

	// report_generation: simulates report creation
	sched.Register("report_generation", func(ctx context.Context, task *scheduler.Task) (map[string]any, error) {
		reportType, _ := task.Payload["report_type"].(string)

		time.Sleep(time.Duration(300+rand.Intn(700)) * time.Millisecond)

		// Simulate occasional failures (20% failure rate) for retry demo
		if rand.Float32() < 0.2 {
			return nil, fmt.Errorf("report service temporarily unavailable")
		}

		return map[string]any{
			"report_id":   fmt.Sprintf("rpt_%d", time.Now().UnixMilli()),
			"report_type": reportType,
			"pages":       rand.Intn(50) + 5,
			"generated":   true,
		}, nil
	})

	// cache_warmup: simulates cache population
	sched.Register("cache_warmup", func(ctx context.Context, task *scheduler.Task) (map[string]any, error) {
		region, _ := task.Payload["region"].(string)
		keys := rand.Intn(500) + 50

		time.Sleep(time.Duration(100+rand.Intn(300)) * time.Millisecond)

		return map[string]any{
			"region":      region,
			"keys_warmed": keys,
		}, nil
	})

	// database_cleanup: simulates old record deletion
	sched.Register("database_cleanup", func(ctx context.Context, task *scheduler.Task) (map[string]any, error) {
		table, _ := task.Payload["table"].(string)
		deleted := rand.Intn(1000)

		time.Sleep(time.Duration(150+rand.Intn(200)) * time.Millisecond)

		return map[string]any{
			"table":           table,
			"records_deleted": deleted,
		}, nil
	})
}

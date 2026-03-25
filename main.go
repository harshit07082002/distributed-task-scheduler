package main

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/example/task-scheduler/api"
	"github.com/example/task-scheduler/scheduler"
	"github.com/example/task-scheduler/storage"
)

func initStore(log *slog.Logger) scheduler.TaskStore {
	host := os.Getenv("REDIS_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("REDIS_PORT")
	if port == "" {
		port = "6379"
	}
	password := os.Getenv("REDIS_PASSWORD")

	store, err := storage.NewRedisStore(host+":"+port, password, 0)
	if err != nil {
		log.Warn("redis unavailable, falling back to memory store", "error", err)
		mem, _ := storage.NewMemoryStore("data/tasks.json")
		return mem
	}
	log.Info("connected to redis", "addr", host+":"+port)
	return store
}

func main() {
	// ── Logger ──────────────────────────────────────────────────────────
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// ── Store ────────────────────────────────────────────────────────────
	store := initStore(log)

	// ── Scheduler ────────────────────────────────────────────────────────
	cfg := scheduler.Config{
		WorkerCount:     5,
		PollInterval:    200 * time.Millisecond,
		MaxQueueSize:    500,
		ShutdownTimeout: 30 * time.Second,
	}
	sched := scheduler.New(cfg, store, log)

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

	// ── Submit Demo Tasks ────────────────────────────────────────────────
	submitDemoTasks(sched, log)

	// ── API Server ───────────────────────────────────────────────────────
	server := api.NewServer(":8080", sched, log)
	go func() {
		fmt.Println()
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println("  Distributed Task Scheduler  |  http://localhost:8080")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println()
		fmt.Println("API Routes:")
		fmt.Println(api.RouterInfo())
		fmt.Println()
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
			"table":          table,
			"records_deleted": deleted,
		}, nil
	})
}

// submitDemoTasks queues a variety of tasks to demonstrate features
func submitDemoTasks(sched *scheduler.Scheduler, log *slog.Logger) {
	log.Info("submitting demo tasks...")

	// 1. Immediate high-priority notification
	t1, _ := sched.Submit("Welcome Email", "email_notification",
		map[string]any{"to": "alice@example.com", "subject": "Welcome!"},
		scheduler.WithPriority(scheduler.PriorityHigh),
		scheduler.WithTags("email", "onboarding"),
		scheduler.WithMetadata("environment", "production"),
	)
	log.Info("submitted high-priority task", "task_id", t1.ID)

	// 2. Scheduled task (5 seconds from now)
	t2, _ := sched.Submit("Sales Report", "report_generation",
		map[string]any{"report_type": "sales_monthly"},
		scheduler.WithSchedule(time.Now().Add(5*time.Second)),
		scheduler.WithRetry(3, 2*time.Second, 2.0),
		scheduler.WithTimeout(10*time.Second),
		scheduler.WithTags("reports", "finance"),
	)
	log.Info("scheduled task in 5s", "task_id", t2.ID)

	// 3. Data export with retry
	t3, _ := sched.Submit("Customer Data Export", "data_export",
		map[string]any{"format": "csv", "table": "customers"},
		scheduler.WithRetry(3, 3*time.Second, 1.5),
		scheduler.WithTags("export", "data"),
	)
	log.Info("submitted export task with retries", "task_id", t3.ID)

	// 4. Multiple cache warmup tasks (different priorities)
	for i, region := range []string{"us-east", "eu-west", "ap-south"} {
		p := scheduler.PriorityNormal
		if i == 0 {
			p = scheduler.PriorityHigh
		}
		task, _ := sched.Submit(
			fmt.Sprintf("Cache Warmup [%s]", region),
			"cache_warmup",
			map[string]any{"region": region},
			scheduler.WithPriority(p),
			scheduler.WithTags("cache", "infrastructure"),
		)
		log.Info("submitted cache warmup", "region", region, "task_id", task.ID)
	}

	// 5. Recurring cleanup (every minute for demo; use @daily in production)
	t5, _ := sched.Submit("Session Cleanup", "database_cleanup",
		map[string]any{"table": "user_sessions"},
		scheduler.WithCron("* * * * *"), // every minute
		scheduler.WithPriority(scheduler.PriorityLow),
		scheduler.WithTags("maintenance", "database"),
	)
	log.Info("submitted recurring cron task", "task_id", t5.ID, "cron", "* * * * *")

	// 6. Dependent task (depends on t3 completing — will fail here as demo)
	// Uncomment this after t3 completes to see it succeed
	// sched.Submit("Post-Export Notification", "email_notification",
	//     map[string]any{"to": "ops@example.com", "subject": "Export ready"},
	//     scheduler.WithDependencies(t3.ID),
	// )

	log.Info("all demo tasks submitted")
}

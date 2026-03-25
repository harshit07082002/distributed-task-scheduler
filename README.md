# Distributed Task Scheduler in Go

A production-ready, distributed task scheduling system built in Go — featuring a priority queue, worker pool, cron support, retries with exponential backoff, and a REST API.

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        REST API (port 8080)                     │
│  POST /api/tasks  GET /api/tasks  GET /api/stats  DELETE /...   │
└────────────────────────────┬────────────────────────────────────┘
                             │
┌────────────────────────────▼────────────────────────────────────┐
│                         Scheduler                               │
│                                                                 │
│  ┌──────────────┐    ┌──────────────┐    ┌───────────────────┐  │
│  │ Priority     │    │   Cron       │    │   Dependency      │  │
│  │ Task Queue   │    │   Engine     │    │   Resolver        │  │
│  │ (min-heap)   │    │   (parser)   │    │                   │  │
│  └──────┬───────┘    └──────┬───────┘    └─────────┬─────────┘  │
│         │                  │                       │            │
│  ┌──────▼───────────────────▼───────────────────────▼─────────┐ │
│  │                    Dispatch Loop                           │ │
│  └──────────────────────────┬──────────────────────────────────┘ │
│                             │                                   │
│  ┌──────────────────────────▼──────────────────────────────────┐ │
│  │                   Worker Pool (N workers)                  │ │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐     │ │
│  │  │ Worker 1 │ │ Worker 2 │ │ Worker 3 │ │ Worker N │ ... │ │
│  │  └──────────┘ └──────────┘ └──────────┘ └──────────┘     │ │
│  └──────────────────────────┬──────────────────────────────────┘ │
│                             │                                   │
│  ┌──────────────────────────▼──────────────────────────────────┐ │
│  │              Result Handler + Retry Engine                 │ │
│  └──────────────────────────┬──────────────────────────────────┘ │
└────────────────────────────┬────────────────────────────────────┘
                             │
┌────────────────────────────▼────────────────────────────────────┐
│               Task Store (Memory + JSON Persistence)            │
│                      data/tasks.json                            │
└─────────────────────────────────────────────────────────────────┘
```

## Features

| Feature | Description |
|---|---|
| **Priority Queue** | Min-heap; tasks sorted by priority (high→low) then scheduled time |
| **Worker Pool** | Configurable N concurrent workers with graceful shutdown |
| **Cron Scheduling** | Full cron syntax + presets (`@hourly`, `@daily`, `@weekly`) |
| **Retry + Backoff** | Configurable max retries with exponential backoff |
| **Scheduled Tasks** | Submit tasks to run at a specific future time |
| **Task Dependencies** | Task A can depend on Task B completing first |
| **Timeouts** | Per-task execution timeouts |
| **REST API** | Full CRUD API with filtering, stats, and health check |
| **Persistence** | JSON file persistence; tasks survive restarts |
| **Recovery** | In-flight tasks re-queued on startup |
| **Hooks** | `OnTaskComplete` / `OnTaskFail` callbacks |
| **Tagging** | Tag tasks for grouping and filtering |

## Quick Start

### Prerequisites

- Go 1.22+

### Run

```bash
git clone <repo>
cd distributed-task-scheduler
go mod tidy
go run .
```

The server starts on **http://localhost:8080** and submits several demo tasks automatically.

## API Reference

### Submit a Task

```bash
curl -X POST http://localhost:8080/api/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Send Welcome Email",
    "type": "email_notification",
    "payload": {
      "to": "user@example.com",
      "subject": "Welcome!"
    },
    "priority": 10,
    "max_retries": 3,
    "retry_delay": "5s",
    "timeout": "30s",
    "tags": ["email", "onboarding"]
  }'
```

### Schedule a Future Task

```bash
curl -X POST http://localhost:8080/api/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Monthly Report",
    "type": "report_generation",
    "payload": {"report_type": "monthly"},
    "scheduled_at": "2026-01-01T09:00:00Z"
  }'
```

### Create a Recurring Cron Task

```bash
curl -X POST http://localhost:8080/api/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Daily Cleanup",
    "type": "database_cleanup",
    "payload": {"table": "sessions"},
    "cron_expr": "@daily"
  }'
```

### List Tasks

```bash
# All tasks
curl http://localhost:8080/api/tasks

# Filter by status: pending | running | completed | failed | retrying | cancelled
curl "http://localhost:8080/api/tasks?status=completed"
```

### Get Task Details

```bash
curl http://localhost:8080/api/tasks/{task_id}
```

### Cancel a Task

```bash
curl -X DELETE http://localhost:8080/api/tasks/{task_id}
```

### Scheduler Stats

```bash
curl http://localhost:8080/api/stats
```

## Cron Expression Format

```
┌──────── minute (0-59)
│ ┌────── hour (0-23)
│ │ ┌──── day of month (1-31)
│ │ │ ┌── month (1-12)
│ │ │ │ ┌ day of week (0-6, Sunday=0)
│ │ │ │ │
* * * * *
```

**Supported syntax:**
- `*` — any value
- `*/n` — every N units  (`*/5` = every 5 minutes)
- `n-m` — range  (`1-5` = Mon-Fri)
- `n,m` — list  (`0,6` = Sunday and Saturday)

**Presets:**
- `@yearly` / `@annually` — once a year (Jan 1 midnight)
- `@monthly` — first day of each month
- `@weekly` — every Sunday midnight
- `@daily` / `@midnight` — every day midnight
- `@hourly` — every hour

## Task Priorities

```go
PriorityLow    = 1
PriorityNormal = 5  // default
PriorityHigh   = 10
```

## Registering Custom Handlers

```go
sched.Register("send_sms", func(ctx context.Context, task *scheduler.Task) (map[string]any, error) {
    phone := task.Payload["phone"].(string)
    msg   := task.Payload["message"].(string)

    // Your implementation here
    err := smsClient.Send(phone, msg)
    if err != nil {
        return nil, err // triggers retry if configured
    }

    return map[string]any{"sid": "SM123456"}, nil
})
```

## Configuration

```go
cfg := scheduler.Config{
    WorkerCount:     10,              // parallel workers
    PollInterval:    100 * time.Millisecond,
    MaxQueueSize:    1000,
    ShutdownTimeout: 30 * time.Second,
}
```

## Project Structure

```
distributed-task-scheduler/
├── main.go               # Entry point, demo tasks, handler registration
├── go.mod
├── scheduler/
│   ├── task.go           # Task model, options, types
│   ├── queue.go          # Thread-safe priority queue (heap)
│   ├── worker.go         # Worker pool implementation
│   ├── cron.go           # Cron expression parser
│   └── scheduler.go      # Main orchestrator
├── api/
│   └── server.go         # HTTP REST API
├── storage/
│   └── store.go          # In-memory + JSON persistence
└── examples/
    └── api_test.sh       # cURL demo script
```

## Extension Points

- **Storage**: Implement `scheduler.TaskStore` to use Redis, PostgreSQL, or any backend
- **Handlers**: Call `sched.Register("type", handler)` for any task type
- **Hooks**: Use `OnTaskComplete` / `OnTaskFail` for notifications, metrics, etc.
- **Distribution**: To distribute across nodes, replace the `TaskStore` with a shared backend (Redis/Postgres) and run multiple instances

## Running the Test Script

```bash
chmod +x examples/api_test.sh
./examples/api_test.sh
```

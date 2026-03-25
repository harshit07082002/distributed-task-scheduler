package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/example/task-scheduler/scheduler"
	"github.com/redis/go-redis/v9"
)

const scheduledQueueKey = "tasks:scheduled_queue"

// popDueScript atomically gets and removes the earliest task with score <= now
var popDueScript = redis.NewScript(`
	local items = redis.call('ZRANGEBYSCORE', KEYS[1], '-inf', ARGV[1], 'LIMIT', 0, 1)
	if #items == 0 then return nil end
	redis.call('ZREM', KEYS[1], items[1])
	return items[1]
`)

const (
	taskKeyPrefix = "task:"
	allTasksKey   = "tasks:all"
	statusKeyFmt  = "tasks:status:%s"
)

// RedisStore implements TaskStore backed by Redis
type RedisStore struct {
	client *redis.Client
	ctx    context.Context
}

// NewRedisStore creates a RedisStore and verifies the connection
func NewRedisStore(addr, password string, db int) (*RedisStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	return &RedisStore{client: client, ctx: ctx}, nil
}

func (r *RedisStore) Save(task *scheduler.Task) error {
	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("failed to marshal task: %w", err)
	}

	key := taskKeyPrefix + task.ID

	// Look up old status to keep status index consistent
	pipe := r.client.Pipeline()
	if old, err := r.Get(task.ID); err == nil && old.Status != task.Status {
		pipe.SRem(r.ctx, fmt.Sprintf(statusKeyFmt, old.Status), task.ID)
	}

	pipe.Set(r.ctx, key, data, 0)
	pipe.SAdd(r.ctx, allTasksKey, task.ID)
	pipe.SAdd(r.ctx, fmt.Sprintf(statusKeyFmt, task.Status), task.ID)

	_, err = pipe.Exec(r.ctx)
	return err
}

func (r *RedisStore) Get(id string) (*scheduler.Task, error) {
	data, err := r.client.Get(r.ctx, taskKeyPrefix+id).Bytes()
	if err == redis.Nil {
		return nil, fmt.Errorf("task %q not found", id)
	}
	if err != nil {
		return nil, err
	}

	var task scheduler.Task
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, fmt.Errorf("failed to unmarshal task: %w", err)
	}
	return &task, nil
}

func (r *RedisStore) GetAll() ([]*scheduler.Task, error) {
	ids, err := r.client.SMembers(r.ctx, allTasksKey).Result()
	if err != nil {
		return nil, err
	}
	return r.fetchTasks(ids)
}

func (r *RedisStore) Delete(id string) error {
	task, err := r.Get(id)
	if err != nil {
		return err
	}

	pipe := r.client.Pipeline()
	pipe.Del(r.ctx, taskKeyPrefix+id)
	pipe.SRem(r.ctx, allTasksKey, id)
	pipe.SRem(r.ctx, fmt.Sprintf(statusKeyFmt, task.Status), id)
	_, err = pipe.Exec(r.ctx)
	return err
}

func (r *RedisStore) GetByStatus(status scheduler.TaskStatus) ([]*scheduler.Task, error) {
	ids, err := r.client.SMembers(r.ctx, fmt.Sprintf(statusKeyFmt, status)).Result()
	if err != nil {
		return nil, err
	}
	return r.fetchTasks(ids)
}

// Enqueue adds a task to the sorted set with score = ScheduledAt nanoseconds
func (r *RedisStore) Enqueue(task *scheduler.Task) error {
	return r.client.ZAdd(r.ctx, scheduledQueueKey, redis.Z{
		Score:  float64(task.ScheduledAt.UnixNano()),
		Member: task.ID,
	}).Err()
}

// PopDue atomically removes and returns the earliest task due by now; nil if none ready
func (r *RedisStore) PopDue(now time.Time) (*scheduler.Task, error) {
	maxScore := strconv.FormatInt(now.UnixNano(), 10)
	result, err := popDueScript.Run(r.ctx, r.client, []string{scheduledQueueKey}, maxScore).Result()
	if err == redis.Nil || result == nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	id, ok := result.(string)
	if !ok {
		return nil, nil
	}
	return r.Get(id)
}

// fetchTasks retrieves multiple tasks by ID in a single pipeline
func (r *RedisStore) fetchTasks(ids []string) ([]*scheduler.Task, error) {
	if len(ids) == 0 {
		return []*scheduler.Task{}, nil
	}

	pipe := r.client.Pipeline()
	cmds := make([]*redis.StringCmd, len(ids))
	for i, id := range ids {
		cmds[i] = pipe.Get(r.ctx, taskKeyPrefix+id)
	}
	pipe.Exec(r.ctx) //nolint:errcheck

	tasks := make([]*scheduler.Task, 0, len(ids))
	for _, cmd := range cmds {
		data, err := cmd.Bytes()
		if err != nil {
			continue
		}
		var task scheduler.Task
		if err := json.Unmarshal(data, &task); err == nil {
			tasks = append(tasks, &task)
		}
	}
	return tasks, nil
}

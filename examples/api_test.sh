#!/usr/bin/env bash
# examples/api_test.sh - Test the Task Scheduler REST API
# Usage: ./examples/api_test.sh

BASE="http://localhost:8080"
COLOR_GREEN='\033[0;32m'
COLOR_BLUE='\033[0;34m'
COLOR_YELLOW='\033[1;33m'
COLOR_RESET='\033[0m'

print_section() {
  echo -e "\n${COLOR_BLUE}══════════════════════════════════════${COLOR_RESET}"
  echo -e "${COLOR_BLUE} $1${COLOR_RESET}"
  echo -e "${COLOR_BLUE}══════════════════════════════════════${COLOR_RESET}"
}

print_ok() { echo -e "${COLOR_GREEN}✓ $1${COLOR_RESET}"; }
print_info() { echo -e "${COLOR_YELLOW}→ $1${COLOR_RESET}"; }

# ── Health Check ──────────────────────────────────────────────────────────────
print_section "Health Check"
curl -s "$BASE/api/health" | jq .
print_ok "health endpoint OK"

# ── Submit Immediate Task ─────────────────────────────────────────────────────
print_section "Submit High-Priority Email Task"
TASK1=$(curl -s -X POST "$BASE/api/tasks" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Welcome Email",
    "type": "email_notification",
    "payload": {"to": "bob@example.com", "subject": "Welcome aboard!"},
    "priority": 10,
    "max_retries": 2,
    "retry_delay": "3s",
    "tags": ["email", "onboarding"],
    "metadata": {"env": "production"}
  }')
echo "$TASK1" | jq .
TASK1_ID=$(echo "$TASK1" | jq -r '.id')
print_ok "created task: $TASK1_ID"

# ── Submit Scheduled Task ─────────────────────────────────────────────────────
print_section "Submit Scheduled Task (10 seconds from now)"
FUTURE=$(date -u -d '+10 seconds' '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || \
         date -u -v +10S '+%Y-%m-%dT%H:%M:%SZ')
TASK2=$(curl -s -X POST "$BASE/api/tasks" \
  -H "Content-Type: application/json" \
  -d "{
    \"name\": \"Monthly Report\",
    \"type\": \"report_generation\",
    \"payload\": {\"report_type\": \"monthly_summary\"},
    \"scheduled_at\": \"$FUTURE\",
    \"timeout\": \"30s\",
    \"tags\": [\"reports\"]
  }")
echo "$TASK2" | jq .
print_ok "scheduled task created"

# ── Submit Recurring Task ─────────────────────────────────────────────────────
print_section "Submit Recurring Cron Task"
TASK3=$(curl -s -X POST "$BASE/api/tasks" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Hourly Cache Refresh",
    "type": "cache_warmup",
    "payload": {"region": "us-east"},
    "cron_expr": "@hourly",
    "priority": 5,
    "tags": ["cache", "recurring"]
  }')
echo "$TASK3" | jq .
TASK3_ID=$(echo "$TASK3" | jq -r '.id')
print_ok "cron task: $TASK3_ID"

# ── List All Tasks ────────────────────────────────────────────────────────────
print_section "List All Tasks"
curl -s "$BASE/api/tasks" | jq '.count, .tasks[].name'
print_ok "listed tasks"

# ── List by Status ────────────────────────────────────────────────────────────
print_section "List Completed Tasks"
curl -s "$BASE/api/tasks?status=completed" | jq '.count'

# ── Get Task Detail ───────────────────────────────────────────────────────────
print_section "Get Task by ID"
curl -s "$BASE/api/tasks/$TASK1_ID" | jq '{id, name, status, priority, tags}'

# ── Stats ─────────────────────────────────────────────────────────────────────
print_section "Scheduler Statistics"
curl -s "$BASE/api/stats" | jq .

# ── Cancel a Task ─────────────────────────────────────────────────────────────
print_section "Cancel Recurring Task"
curl -s -X DELETE "$BASE/api/tasks/$TASK3_ID" | jq .
print_ok "cancelled task $TASK3_ID"

print_section "Done"
print_info "Run 'curl -s $BASE/api/stats | jq .' to see final stats"

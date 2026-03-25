package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CronSchedule represents a parsed cron expression
// Format: minute hour dayOfMonth month dayOfWeek
// Supports: * (any), */n (every n), n-m (range), n,m (list)
// Presets: @hourly, @daily, @weekly, @monthly, @yearly, @every_Xs
type CronSchedule struct {
	Minutes     []int
	Hours       []int
	DaysOfMonth []int
	Months      []int
	DaysOfWeek  []int
	expr        string
}

var cronPresets = map[string]string{
	"@yearly":   "0 0 1 1 *",
	"@annually": "0 0 1 1 *",
	"@monthly":  "0 0 1 * *",
	"@weekly":   "0 0 * * 0",
	"@daily":    "0 0 * * *",
	"@midnight": "0 0 * * *",
	"@hourly":   "0 * * * *",
}

// ParseCron parses a cron expression into a CronSchedule
func ParseCron(expr string) (*CronSchedule, error) {
	expr = strings.TrimSpace(expr)

	// Handle presets
	if preset, ok := cronPresets[expr]; ok {
		expr = preset
	}

	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return nil, fmt.Errorf("invalid cron expression %q: expected 5 fields, got %d", expr, len(parts))
	}

	s := &CronSchedule{expr: expr}
	var err error

	if s.Minutes, err = parseCronField(parts[0], 0, 59); err != nil {
		return nil, fmt.Errorf("invalid minutes field: %w", err)
	}
	if s.Hours, err = parseCronField(parts[1], 0, 23); err != nil {
		return nil, fmt.Errorf("invalid hours field: %w", err)
	}
	if s.DaysOfMonth, err = parseCronField(parts[2], 1, 31); err != nil {
		return nil, fmt.Errorf("invalid day-of-month field: %w", err)
	}
	if s.Months, err = parseCronField(parts[3], 1, 12); err != nil {
		return nil, fmt.Errorf("invalid month field: %w", err)
	}
	if s.DaysOfWeek, err = parseCronField(parts[4], 0, 6); err != nil {
		return nil, fmt.Errorf("invalid day-of-week field: %w", err)
	}

	return s, nil
}

// Next returns the next time the cron expression fires after t
func (s *CronSchedule) Next(t time.Time) time.Time {
	// Start from the next minute
	t = t.Add(time.Minute).Truncate(time.Minute)

	// Try up to 4 years to find next match
	end := t.Add(4 * 365 * 24 * time.Hour)

	for t.Before(end) {
		if !contains(s.Months, int(t.Month())) {
			// Advance to first day of next valid month
			t = advanceMonth(t, s.Months)
			continue
		}
		if !contains(s.DaysOfMonth, t.Day()) || !contains(s.DaysOfWeek, int(t.Weekday())) {
			t = t.AddDate(0, 0, 1).Truncate(24 * time.Hour)
			continue
		}
		if !contains(s.Hours, t.Hour()) {
			t = t.Add(time.Hour).Truncate(time.Hour)
			continue
		}
		if !contains(s.Minutes, t.Minute()) {
			t = t.Add(time.Minute)
			continue
		}
		return t
	}

	return time.Time{} // No next occurrence found
}

func advanceMonth(t time.Time, months []int) time.Time {
	next := time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
	for !contains(months, int(next.Month())) {
		next = time.Date(next.Year(), next.Month()+1, 1, 0, 0, 0, 0, next.Location())
		if next.Year() > t.Year()+4 {
			break
		}
	}
	return next
}

func contains(slice []int, val int) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

func parseCronField(field string, min, max int) ([]int, error) {
	if field == "*" {
		result := make([]int, max-min+1)
		for i := range result {
			result[i] = min + i
		}
		return result, nil
	}

	var result []int
	parts := strings.Split(field, ",")

	for _, part := range parts {
		// Handle step: */5 or 1-5/2
		step := 1
		if idx := strings.Index(part, "/"); idx != -1 {
			var err error
			step, err = strconv.Atoi(part[idx+1:])
			if err != nil || step < 1 {
				return nil, fmt.Errorf("invalid step value in %q", field)
			}
			part = part[:idx]
		}

		var rangeMin, rangeMax int

		if part == "*" {
			rangeMin, rangeMax = min, max
		} else if idx := strings.Index(part, "-"); idx != -1 {
			var err error
			rangeMin, err = strconv.Atoi(part[:idx])
			if err != nil {
				return nil, fmt.Errorf("invalid range in %q", field)
			}
			rangeMax, err = strconv.Atoi(part[idx+1:])
			if err != nil {
				return nil, fmt.Errorf("invalid range in %q", field)
			}
		} else {
			n, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid value %q", part)
			}
			rangeMin, rangeMax = n, n
		}

		if rangeMin < min || rangeMax > max || rangeMin > rangeMax {
			return nil, fmt.Errorf("value out of range [%d,%d] in %q", min, max, field)
		}

		for v := rangeMin; v <= rangeMax; v += step {
			result = append(result, v)
		}
	}

	return result, nil
}

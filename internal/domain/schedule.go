package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// maxCronBytes bounds a cron expression.
const maxCronBytes = 255

// OverlapPolicy is what a tick does while the schedule's last Run has not ended.
type OverlapPolicy string

const (
	// OverlapAllow creates the Run anyway.
	OverlapAllow OverlapPolicy = "allow"
	// OverlapSkip creates no Run for the tick.
	OverlapSkip OverlapPolicy = "skip"
)

// Schedule is the cron schedule of a Task, read in UTC.
type Schedule struct {
	Cron    string
	Overlap OverlapPolicy
}

// cronParser reads five fields, or six with seconds first. Descriptors such as @daily
// are refused: every schedule is a plain cron expression.
var cronParser = cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// Cron is a parsed cron expression.
type Cron struct {
	expr     string
	schedule cron.Schedule
}

// ParseCron reads expr in UTC. It refuses a time zone prefix, since every schedule is
// read in UTC, and an expression that never fires, such as February 30. The fields are
// joined by single spaces, so two spellings that differ only in spaces are one
// expression.
func ParseCron(expr string) (Cron, error) {
	if len(expr) > maxCronBytes {
		return Cron{}, fmt.Errorf("cron must be at most %d bytes", maxCronBytes)
	}

	fields := strings.Fields(expr)
	if len(fields) == 0 {
		return Cron{}, errors.New("cron is empty")
	}

	if strings.Contains(fields[0], "=") {
		return Cron{}, errors.New("cron must not set a time zone: every schedule is read in UTC")
	}

	normalized := strings.Join(fields, " ")
	schedule, err := cronParser.Parse(normalized)
	if err != nil {
		return Cron{}, fmt.Errorf("parse cron: %w", err)
	}

	c := Cron{expr: normalized, schedule: schedule}
	if c.Next(time.Now()).IsZero() {
		return Cron{}, errors.New("cron never fires")
	}

	return c, nil
}

// String returns the expression with its fields joined by single spaces.
func (c Cron) String() string { return c.expr }

// Next returns the first time after t that the expression fires, in UTC, or the zero time
// when it never fires.
func (c Cron) Next(t time.Time) time.Time {
	return c.schedule.Next(t.UTC())
}

// Validate checks the schedule's cron and overlap policy.
func (s Schedule) Validate() error {
	if _, err := ParseCron(s.Cron); err != nil {
		return err
	}

	if s.Overlap != OverlapAllow && s.Overlap != OverlapSkip {
		return fmt.Errorf("overlap must be %s or %s", OverlapAllow, OverlapSkip)
	}

	return nil
}

// ScheduleTicks is what one sweep of the due schedules did.
type ScheduleTicks struct {
	// Fired counts the ticks that created a Run.
	Fired int
	// Skipped counts the ticks that created none, because the schedule skips overlaps and
	// its last Run had not ended.
	Skipped int
	// Broken counts the schedules whose stored cron could not be parsed. They stay due.
	Broken int
}

package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCron(t *testing.T) {
	at := time.Date(2026, 11, 10, 10, 0, 7, 0, time.UTC)

	tests := []struct {
		expr       string
		normalized string
		next       time.Time
	}{
		{expr: "*/15 * * * *", normalized: "*/15 * * * *", next: time.Date(2026, 11, 10, 10, 15, 0, 0, time.UTC)},
		{expr: "*/10 * * * * *", normalized: "*/10 * * * * *", next: time.Date(2026, 11, 10, 10, 0, 10, 0, time.UTC)},
		{expr: "  0   2,14 * * * ", normalized: "0 2,14 * * *", next: time.Date(2026, 11, 10, 14, 0, 0, 0, time.UTC)},
		{expr: "0 0 1 * MON", normalized: "0 0 1 * MON", next: time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			c, err := ParseCron(tt.expr)
			require.NoError(t, err)
			assert.Equal(t, tt.normalized, c.String())
			assert.Equal(t, tt.next, c.Next(at))
		})
	}
}

func TestCronNextIsInUTC(t *testing.T) {
	c, err := ParseCron("0 9 * * *")
	require.NoError(t, err)

	// 10:00 at UTC+7 is 03:00 UTC, so the next 09:00 UTC is the same day.
	loc := time.FixedZone("UTC+7", 7*3600)
	next := c.Next(time.Date(2026, 11, 10, 10, 0, 0, 0, loc))

	assert.Equal(t, time.UTC, next.Location())
	assert.Equal(t, time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC), next)
}

func TestParseCronRefuses(t *testing.T) {
	tests := []struct {
		expr string
		want string
	}{
		{expr: "", want: "cron is empty"},
		{expr: "* * *", want: "parse cron: expected 5 to 6 fields, found 3: [* * *]"},
		{expr: "60 * * * *", want: "parse cron: end of range (60) above maximum (59): 60"},
		{expr: "@daily", want: "parse cron: parser does not accept descriptors: @daily"},
		{expr: "TZ=Asia/Ho_Chi_Minh 0 9 * * *", want: "cron must not set a time zone: every schedule is read in UTC"},
		{expr: "CRON_TZ=UTC 0 9 * * *", want: "cron must not set a time zone: every schedule is read in UTC"},
		{expr: "0 0 30 2 *", want: "cron never fires"},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			_, err := ParseCron(tt.expr)
			assert.EqualError(t, err, tt.want)
		})
	}
}

func TestScheduleValidate(t *testing.T) {
	assert.NoError(t, Schedule{Cron: "0 * * * *", Overlap: OverlapAllow}.Validate())
	assert.NoError(t, Schedule{Cron: "0 * * * *", Overlap: OverlapSkip}.Validate())
	assert.EqualError(t, Schedule{Cron: "0 * * * *"}.Validate(), "overlap must be allow or skip")
	assert.EqualError(t, Schedule{Cron: "nope", Overlap: OverlapAllow}.Validate(), "parse cron: expected 5 to 6 fields, found 1: [nope]")
}

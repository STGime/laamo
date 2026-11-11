// Package schedule computes when a scheduled helper runs next, and keeps a
// helper's schedule trigger in step with its stack (the Every… brick).
//
// Next must agree with the database's next_schedule_time
// (migrations/0005): the API sets triggers.next_run_at with Next when a
// helper is saved or switched on, the scheduler (enqueue_due_triggers)
// advances it with the SQL version after each run.
package schedule

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// Config is the Every… brick's settings (triggers.config).
type Config struct {
	Freq string   `json:"freq"` // daily, weekdays, weekly
	Time string   `json:"time"` // HH:MM, local to the workspace
	Days []string `json:"days,omitempty"`
}

// ErrNever: the schedule can never fire (weekly with no days).
var ErrNever = errors.New("schedule never fires")

var dayIndex = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}

// Next returns the first run strictly after from, in the timezone tz (an
// IANA name). It returns ErrNever when the schedule can't fire.
func Next(cfg Config, tz string, from time.Time) (time.Time, error) {
	loc, err := time.LoadLocation(tz)
	if err != nil || tz == "" || tz == "Local" {
		return time.Time{}, fmt.Errorf("schedule: unknown timezone %q", tz)
	}
	freq := cfg.Freq
	if freq == "" {
		freq = "daily"
	}
	if freq != "daily" && freq != "weekdays" && freq != "weekly" {
		return time.Time{}, fmt.Errorf("schedule: unknown freq %q", freq)
	}
	hhmm := cfg.Time
	if hhmm == "" {
		hhmm = "08:00"
	}
	at, err := time.Parse("15:04", hhmm)
	if err != nil {
		return time.Time{}, fmt.Errorf("schedule: bad time %q", hhmm)
	}

	local := from.In(loc)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	for i := range 8 {
		d := day.AddDate(0, 0, i)
		wd := d.Weekday()
		ok := freq == "daily" ||
			(freq == "weekdays" && wd >= time.Monday && wd <= time.Friday) ||
			(freq == "weekly" && slices.ContainsFunc(cfg.Days, func(s string) bool { w, known := dayIndex[s]; return known && w == wd }))
		if !ok {
			continue
		}
		candidate := wallClock(d, at.Hour(), at.Minute(), loc)
		if candidate.After(from) {
			return candidate.UTC(), nil
		}
	}
	return time.Time{}, ErrNever
}

// wallClock returns hh:mm local time on day d. A time that doesn't exist
// that day (inside the spring-forward gap) moves forward by the gap, like
// Postgres does; Go's time.Date doesn't promise which side it picks.
func wallClock(d time.Time, hh, mm int, loc *time.Location) time.Time {
	t := time.Date(d.Year(), d.Month(), d.Day(), hh, mm, 0, 0, loc)
	if t.Hour() == hh && t.Minute() == mm {
		return t
	}
	// In the gap: interpret with the offset in effect before it.
	_, before := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc).Zone()
	return time.Date(d.Year(), d.Month(), d.Day(), hh, mm, 0, 0, time.FixedZone("", before)).In(loc)
}

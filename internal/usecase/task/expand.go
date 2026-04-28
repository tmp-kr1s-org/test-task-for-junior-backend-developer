package task

import (
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
)

// truncateToDay returns t at 00:00:00 in its own location. The result is used
// only for date arithmetic — wall-clock comparisons happen elsewhere.
func truncateToDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// ExpandSeries materializes the dates a series produces inside [from, to]
// (both inclusive), respecting the rule's own start/end window. The result is
// pure: same input → same output, no DB or clock involvement.
func ExpandSeries(rule taskdomain.RepeatRule, from, to time.Time) []time.Time {
	from = truncateToDay(from)
	to = truncateToDay(to)
	start := truncateToDay(rule.StartDate)

	if from.Before(start) {
		from = start
	}
	if rule.EndDate != nil {
		end := truncateToDay(*rule.EndDate)
		if to.After(end) {
			to = end
		}
	}
	if to.Before(from) {
		return nil
	}

	switch rule.Type {
	case taskdomain.RuleSpecificDates:
		var out []time.Time
		for _, d := range rule.SpecificDates {
			d = truncateToDay(d)
			if !d.Before(from) && !d.After(to) {
				out = append(out, d)
			}
		}
		return out

	case taskdomain.RuleEvenDays:
		return walk(from, to, func(d time.Time) bool { return d.Day()%2 == 0 })

	case taskdomain.RuleOddDays:
		return walk(from, to, func(d time.Time) bool { return d.Day()%2 == 1 })

	case taskdomain.RuleDaily:
		if rule.IntervalDays == nil || *rule.IntervalDays < 1 {
			return nil
		}
		interval := *rule.IntervalDays
		return walk(from, to, func(d time.Time) bool {
			diff := int(d.Sub(start).Hours() / 24)
			return diff >= 0 && diff%interval == 0
		})

	case taskdomain.RuleWeekly:
		set := make(map[int]bool, len(rule.Weekdays))
		for _, w := range rule.Weekdays {
			set[w] = true
		}
		return walk(from, to, func(d time.Time) bool {
			// Mon=1..Sun=7
			wd := int(d.Weekday())
			if wd == 0 {
				wd = 7
			}
			return set[wd]
		})

	case taskdomain.RuleMonthly:
		set := make(map[int]bool, len(rule.MonthDays))
		for _, m := range rule.MonthDays {
			set[m] = true
		}
		return walk(from, to, func(d time.Time) bool { return set[d.Day()] })
	}

	return nil
}

func walk(from, to time.Time, keep func(time.Time) bool) []time.Time {
	var out []time.Time
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		if keep(d) {
			out = append(out, d)
		}
	}
	return out
}

// Merge applies overrides to a series template across the given dates,
// dropping any date that is cancelled. The series base is the Task's own
// fields; overrides patch them per-day.
func Merge(series taskdomain.Task, repeatID int64, dates []time.Time, overrides map[string]taskdomain.Override) []taskdomain.Occurrence {
	out := make([]taskdomain.Occurrence, 0, len(dates))
	for _, d := range dates {
		ov, has := overrides[DateKey(d)]
		if has && ov.IsCancelled {
			continue
		}

		occ := taskdomain.Occurrence{
			Task: series,
			Date: d,
		}
		// Anchor scheduled time-of-day on the occurrence date.
		if series.ScheduledAt != nil {
			st := *series.ScheduledAt
			occ.ScheduledAt = combineDateAndTime(d, st)
		}
		if has {
			if ov.Title != nil {
				occ.Title = *ov.Title
			}
			if ov.Description != nil {
				occ.Description = *ov.Description
			}
			if ov.Status != nil {
				occ.Status = *ov.Status
			}
			if ov.ScheduledAt != nil {
				v := *ov.ScheduledAt
				occ.ScheduledAt = &v
			}
		}
		_ = repeatID
		out = append(out, occ)
	}
	return out
}

func combineDateAndTime(date, t time.Time) *time.Time {
	r := time.Date(date.Year(), date.Month(), date.Day(),
		t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
	return &r
}

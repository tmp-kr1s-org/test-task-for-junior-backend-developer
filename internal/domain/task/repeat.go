package task

import (
	"errors"
	"time"
)

type RuleType string

const (
	RuleDaily         RuleType = "daily"
	RuleWeekly        RuleType = "weekly"
	RuleMonthly       RuleType = "monthly"
	RuleSpecificDates RuleType = "specific_dates"
	RuleEvenDays      RuleType = "even_days"
	RuleOddDays       RuleType = "odd_days"
)

func (t RuleType) Valid() bool {
	switch t {
	case RuleDaily, RuleWeekly, RuleMonthly, RuleSpecificDates, RuleEvenDays, RuleOddDays:
		return true
	default:
		return false
	}
}

// RepeatRule describes how a task series unfolds over time.
// StartDate is required (the anchor); EndDate NULL means open-ended.
// Only the parameter set that matches Type is meaningful; the rest must be empty.
type RepeatRule struct {
	ID            int64
	TaskID        int64
	Type          RuleType
	StartDate     time.Time
	EndDate       *time.Time
	IntervalDays  *int
	Weekdays      []int
	MonthDays     []int
	SpecificDates []time.Time
}

var ErrInvalidRule = errors.New("invalid repeat rule")

func (r *RepeatRule) Validate() error {
	if !r.Type.Valid() {
		return ErrInvalidRule
	}
	if r.EndDate != nil && r.EndDate.Before(r.StartDate) {
		return ErrInvalidRule
	}
	switch r.Type {
	case RuleDaily:
		if r.IntervalDays == nil || *r.IntervalDays < 1 {
			return ErrInvalidRule
		}
		if len(r.Weekdays) > 0 || len(r.MonthDays) > 0 || len(r.SpecificDates) > 0 {
			return ErrInvalidRule
		}
	case RuleWeekly:
		if len(r.Weekdays) == 0 {
			return ErrInvalidRule
		}
		for _, d := range r.Weekdays {
			if d < 1 || d > 7 {
				return ErrInvalidRule
			}
		}
		if r.IntervalDays != nil || len(r.MonthDays) > 0 || len(r.SpecificDates) > 0 {
			return ErrInvalidRule
		}
	case RuleMonthly:
		if len(r.MonthDays) == 0 {
			return ErrInvalidRule
		}
		for _, d := range r.MonthDays {
			if d < 1 || d > 30 {
				return ErrInvalidRule
			}
		}
		if r.IntervalDays != nil || len(r.Weekdays) > 0 || len(r.SpecificDates) > 0 {
			return ErrInvalidRule
		}
	case RuleSpecificDates:
		if len(r.SpecificDates) == 0 {
			return ErrInvalidRule
		}
		if r.IntervalDays != nil || len(r.Weekdays) > 0 || len(r.MonthDays) > 0 {
			return ErrInvalidRule
		}
	case RuleEvenDays, RuleOddDays:
		if r.IntervalDays != nil || len(r.Weekdays) > 0 || len(r.MonthDays) > 0 || len(r.SpecificDates) > 0 {
			return ErrInvalidRule
		}
	}
	return nil
}

// Override is a per-occurrence patch on top of a series.
// Nil pointers mean "inherit from series"; IsCancelled is a tombstone.
type Override struct {
	RepeatID     int64
	Date         time.Time
	Title        *string
	Description  *string
	Status       *Status
	ScheduledAt  *time.Time
	IsCancelled  bool
}

// Occurrence is a single materialized day of a series (or a single non-recurring task).
type Occurrence struct {
	Task
	Date time.Time `json:"date"`
}

package handlers

import (
	"errors"
	"strings"
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
	taskusecase "example.com/taskservice/internal/usecase/task"
)

// jsonDate accepts a YYYY-MM-DD string in JSON. time.Time's default unmarshal
// demands RFC3339, but the contract advertises format=date.
type jsonDate struct{ time.Time }

func (d *jsonDate) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return errors.New("invalid date (want YYYY-MM-DD)")
	}
	d.Time = t
	return nil
}

func (d jsonDate) MarshalJSON() ([]byte, error) {
	return []byte(`"` + d.Format("2006-01-02") + `"`), nil
}

type taskMutationDTO struct {
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Status      taskdomain.Status `json:"status"`
	ScheduledAt *time.Time        `json:"scheduled_at,omitempty"`
	Repeat      *repeatRuleDTO    `json:"repeat,omitempty"`
}

type taskDTO struct {
	ID          int64             `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Status      taskdomain.Status `json:"status"`
	ScheduledAt *time.Time        `json:"scheduled_at,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Repeat      *repeatRuleDTO    `json:"repeat,omitempty"`
}

type repeatRuleDTO struct {
	Type          taskdomain.RuleType `json:"type"`
	StartDate     *jsonDate           `json:"start_date,omitempty"`
	EndDate       *jsonDate           `json:"end_date,omitempty"`
	IntervalDays  *int                `json:"interval_days,omitempty"`
	Weekdays      []int               `json:"weekdays,omitempty"`
	MonthDays     []int               `json:"month_days,omitempty"`
	SpecificDates []jsonDate          `json:"specific_dates,omitempty"`
}

// occurrenceDTO carries both `date` and `scheduled_at` on purpose.
// `date` is the canonical occurrence key (YYYY-MM-DD): it appears in URLs
// (/tasks/{id}/occurrences/{date}), in override lookups, and is the only
// day-bearing field when the task has no time-of-day (scheduled_at == null).
// `scheduled_at` is the full datetime convenience: date + series' time-of-day,
// so clients don't have to splice them together. The two overlap when
// scheduled_at is set, but neither one alone covers both cases.
type occurrenceDTO struct {
	taskDTO
	Date string `json:"date"`
}

type overrideDTO struct {
	Title       *string            `json:"title,omitempty"`
	Description *string            `json:"description,omitempty"`
	Status      *taskdomain.Status `json:"status,omitempty"`
	ScheduledAt *time.Time         `json:"scheduled_at,omitempty"`
}

type forkRequestDTO struct {
	FromDate    string             `json:"from_date"`
	Title       string             `json:"title"`
	Description string             `json:"description"`
	Status      taskdomain.Status  `json:"status"`
	ScheduledAt *time.Time         `json:"scheduled_at,omitempty"`
	Repeat      *forkRepeatRuleDTO `json:"repeat,omitempty"`
}

// forkRepeatRuleDTO mirrors repeatRuleDTO but omits start_date: a fork's
// new series always anchors at from_date, so accepting start_date here would
// silently shadow it. end_date is allowed — a forked series can still be finite.
type forkRepeatRuleDTO struct {
	Type          taskdomain.RuleType `json:"type"`
	EndDate       *jsonDate           `json:"end_date,omitempty"`
	IntervalDays  *int                `json:"interval_days,omitempty"`
	Weekdays      []int               `json:"weekdays,omitempty"`
	MonthDays     []int               `json:"month_days,omitempty"`
	SpecificDates []jsonDate          `json:"specific_dates,omitempty"`
}

func newTaskDTO(task *taskdomain.Task) taskDTO {
	return taskDTO{
		ID:          task.ID,
		Title:       task.Title,
		Description: task.Description,
		Status:      task.Status,
		ScheduledAt: task.ScheduledAt,
		CreatedAt:   task.CreatedAt,
		UpdatedAt:   task.UpdatedAt,
	}
}

func newSeriesDTO(ser *taskusecase.Series) taskDTO {
	dto := newTaskDTO(&ser.Task)
	if ser.Rule != nil {
		dto.Repeat = newRepeatRuleDTO(ser.Rule)
	}
	return dto
}

func newRepeatRuleDTO(rule *taskdomain.RepeatRule) *repeatRuleDTO {
	dto := &repeatRuleDTO{
		Type:         rule.Type,
		StartDate:    &jsonDate{Time: rule.StartDate},
		IntervalDays: rule.IntervalDays,
		Weekdays:     rule.Weekdays,
		MonthDays:    rule.MonthDays,
	}
	if rule.EndDate != nil {
		dto.EndDate = &jsonDate{Time: *rule.EndDate}
	}
	if len(rule.SpecificDates) > 0 {
		dto.SpecificDates = make([]jsonDate, len(rule.SpecificDates))
		for i, d := range rule.SpecificDates {
			dto.SpecificDates[i] = jsonDate{Time: d}
		}
	}
	return dto
}

func newOccurrenceDTO(occ *taskdomain.Occurrence) occurrenceDTO {
	return occurrenceDTO{
		taskDTO: newTaskDTO(&occ.Task),
		Date:    occ.Date.Format("2006-01-02"),
	}
}

func (d *repeatRuleDTO) toInput(scheduledAt *time.Time) *taskusecase.RepeatInput {
	if d == nil {
		return nil
	}
	in := &taskusecase.RepeatInput{
		Type:         d.Type,
		IntervalDays: d.IntervalDays,
		Weekdays:     d.Weekdays,
		MonthDays:    d.MonthDays,
	}
	if d.EndDate != nil {
		end := d.EndDate.Time
		in.EndDate = &end
	}
	if len(d.SpecificDates) > 0 {
		in.SpecificDates = make([]time.Time, len(d.SpecificDates))
		for i, sd := range d.SpecificDates {
			in.SpecificDates[i] = sd.Time
		}
	}
	if d.StartDate != nil {
		in.StartDate = d.StartDate.Time
	} else if scheduledAt != nil {
		in.StartDate = *scheduledAt
	}
	return in
}

func (d *forkRepeatRuleDTO) toInput() *taskusecase.RepeatInput {
	if d == nil {
		return nil
	}
	in := &taskusecase.RepeatInput{
		Type:         d.Type,
		IntervalDays: d.IntervalDays,
		Weekdays:     d.Weekdays,
		MonthDays:    d.MonthDays,
	}
	if d.EndDate != nil {
		end := d.EndDate.Time
		in.EndDate = &end
	}
	if len(d.SpecificDates) > 0 {
		in.SpecificDates = make([]time.Time, len(d.SpecificDates))
		for i, sd := range d.SpecificDates {
			in.SpecificDates[i] = sd.Time
		}
	}
	return in
}

func (d *overrideDTO) toInput() taskusecase.OverrideInput {
	return taskusecase.OverrideInput{
		Title:       d.Title,
		Description: d.Description,
		Status:      d.Status,
		ScheduledAt: d.ScheduledAt,
	}
}

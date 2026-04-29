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
	return []byte(`"` + d.Time.Format("2006-01-02") + `"`), nil
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

type occurrenceDTO struct {
	taskDTO
	Date string `json:"date"`
}

type overrideDTO struct {
	Title       *string            `json:"title,omitempty"`
	Description *string            `json:"description,omitempty"`
	Status      *taskdomain.Status `json:"status,omitempty"`
	ScheduledAt *time.Time         `json:"scheduled_at,omitempty"`
	IsCancelled bool               `json:"is_cancelled,omitempty"`
}

type forkRequestDTO struct {
	FromDate    string            `json:"from_date"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Status      taskdomain.Status `json:"status"`
	ScheduledAt *time.Time        `json:"scheduled_at,omitempty"`
	Repeat      *repeatRuleDTO    `json:"repeat,omitempty"`
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

func (d *overrideDTO) toInput() taskusecase.OverrideInput {
	return taskusecase.OverrideInput{
		Title:       d.Title,
		Description: d.Description,
		Status:      d.Status,
		ScheduledAt: d.ScheduledAt,
		IsCancelled: d.IsCancelled,
	}
}

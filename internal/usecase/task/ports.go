package task

import (
	"context"
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
)

// Series bundles a Task with an optional repeat rule (1:1 LEFT JOIN view).
type Series struct {
	Task taskdomain.Task
	Rule *taskdomain.RepeatRule
}

type Repository interface {
	Create(ctx context.Context, task *taskdomain.Task) (*taskdomain.Task, error)
	GetByID(ctx context.Context, id int64) (*taskdomain.Task, error)
	Update(ctx context.Context, task *taskdomain.Task) (*taskdomain.Task, error)
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context) ([]taskdomain.Task, error)

	CreateWithRepeat(ctx context.Context, task *taskdomain.Task, rule *taskdomain.RepeatRule) (*taskdomain.Task, *taskdomain.RepeatRule, error)
	GetSeriesByTaskID(ctx context.Context, taskID int64) (*Series, error)
	ListSeries(ctx context.Context, from, to *time.Time) ([]Series, error)
	LoadOverrides(ctx context.Context, repeatIDs []int64, from, to time.Time) (map[int64]map[string]taskdomain.Override, error)
	UpsertOverride(ctx context.Context, override taskdomain.Override) error
	ForkSeries(ctx context.Context, taskID int64, fromDate time.Time, newTask *taskdomain.Task, newRule *taskdomain.RepeatRule) (*taskdomain.Task, *taskdomain.RepeatRule, error)
}

type Usecase interface {
	Create(ctx context.Context, input CreateInput) (*taskdomain.Task, error)
	GetByID(ctx context.Context, id int64) (*taskdomain.Task, error)
	Update(ctx context.Context, id int64, input UpdateInput) (*taskdomain.Task, error)
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context) ([]taskdomain.Task, error)

	ListOccurrences(ctx context.Context, from, to time.Time) ([]taskdomain.Occurrence, error)
	UpsertOccurrenceOverride(ctx context.Context, taskID int64, date time.Time, in OverrideInput) error
	CancelOccurrence(ctx context.Context, taskID int64, date time.Time) error
	ForkSeries(ctx context.Context, taskID int64, fromDate time.Time, in UpdateInput, rule *RepeatInput) (*taskdomain.Task, error)
}

type RepeatInput struct {
	Type          taskdomain.RuleType
	StartDate     time.Time
	EndDate       *time.Time
	IntervalDays  *int
	Weekdays      []int
	MonthDays     []int
	SpecificDates []time.Time
}

type CreateInput struct {
	Title       string
	Description string
	Status      taskdomain.Status
	ScheduledAt *time.Time
	Repeat      *RepeatInput
}

type UpdateInput struct {
	Title       string
	Description string
	Status      taskdomain.Status
	ScheduledAt *time.Time
}

type OverrideInput struct {
	Title       *string
	Description *string
	Status      *taskdomain.Status
	ScheduledAt *time.Time
	IsCancelled bool
}

// DateKey formats a date as YYYY-MM-DD (used as map key for overrides).
func DateKey(t time.Time) string {
	return t.Format("2006-01-02")
}

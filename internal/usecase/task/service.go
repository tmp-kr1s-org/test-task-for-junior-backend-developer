package task

import (
	"context"
	"fmt"
	"strings"
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
)

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) *Service {
	return &Service{
		repo: repo,
		now:  func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*taskdomain.Task, error) {
	normalized, err := validateCreateInput(input)
	if err != nil {
		return nil, err
	}

	model := &taskdomain.Task{
		Title:       normalized.Title,
		Description: normalized.Description,
		Status:      normalized.Status,
		ScheduledAt: normalized.ScheduledAt,
	}
	now := s.now()
	model.CreatedAt = now
	model.UpdatedAt = now

	if normalized.Repeat == nil {
		return s.repo.Create(ctx, model)
	}

	rule, err := buildRule(normalized.Repeat, normalized.ScheduledAt)
	if err != nil {
		return nil, err
	}

	created, _, err := s.repo.CreateWithRepeat(ctx, model, rule)
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (s *Service) GetByID(ctx context.Context, id int64) (*taskdomain.Task, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}

	return s.repo.GetByID(ctx, id)
}

func (s *Service) Update(ctx context.Context, id int64, input UpdateInput) (*taskdomain.Task, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}

	normalized, err := validateUpdateInput(input)
	if err != nil {
		return nil, err
	}

	model := &taskdomain.Task{
		ID:          id,
		Title:       normalized.Title,
		Description: normalized.Description,
		Status:      normalized.Status,
		ScheduledAt: normalized.ScheduledAt,
		UpdatedAt:   s.now(),
	}

	updated, err := s.repo.Update(ctx, model)
	if err != nil {
		return nil, err
	}

	return updated, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}

	return s.repo.Delete(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]taskdomain.Task, error) {
	return s.repo.List(ctx)
}

func (s *Service) ListOccurrences(ctx context.Context, from, to time.Time) ([]taskdomain.Occurrence, error) {
	if to.Before(from) {
		return nil, fmt.Errorf("%w: to must be >= from", ErrInvalidInput)
	}

	series, err := s.repo.ListSeries(ctx, &from, &to)
	if err != nil {
		return nil, err
	}

	repeatIDs := make([]int64, 0, len(series))
	for _, ser := range series {
		if ser.Rule != nil {
			repeatIDs = append(repeatIDs, ser.Rule.ID)
		}
	}

	overrides := map[int64]map[string]taskdomain.Override{}
	if len(repeatIDs) > 0 {
		overrides, err = s.repo.LoadOverrides(ctx, repeatIDs, from, to)
		if err != nil {
			return nil, err
		}
	}

	out := make([]taskdomain.Occurrence, 0)
	for _, ser := range series {
		if ser.Rule == nil {
			if ser.Task.ScheduledAt == nil {
				continue
			}
			d := truncateToDay(*ser.Task.ScheduledAt)
			if d.Before(truncateToDay(from)) || d.After(truncateToDay(to)) {
				continue
			}
			out = append(out, taskdomain.Occurrence{Task: ser.Task, Date: d})
			continue
		}

		dates := ExpandSeries(*ser.Rule, from, to)
		merged := Merge(ser.Task, ser.Rule.ID, dates, overrides[ser.Rule.ID])
		out = append(out, merged...)
	}
	return out, nil
}

func (s *Service) UpsertOccurrenceOverride(ctx context.Context, taskID int64, date time.Time, in OverrideInput) error {
	if taskID <= 0 {
		return fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}
	ser, err := s.repo.GetSeriesByTaskID(ctx, taskID)
	if err != nil {
		return err
	}
	if ser.Rule == nil {
		return fmt.Errorf("%w: task is not recurring", ErrInvalidInput)
	}
	day := truncateToDay(date)
	dates := ExpandSeries(*ser.Rule, day, day)
	if len(dates) == 0 {
		return fmt.Errorf("%w: date is not produced by the rule", ErrInvalidInput)
	}
	if in.Status != nil && !in.Status.Valid() {
		return fmt.Errorf("%w: invalid status", ErrInvalidInput)
	}

	ov := taskdomain.Override{
		RepeatID:    ser.Rule.ID,
		Date:        day,
		Title:       in.Title,
		Description: in.Description,
		Status:      in.Status,
		ScheduledAt: in.ScheduledAt,
		IsCancelled: in.IsCancelled,
	}
	return s.repo.UpsertOverride(ctx, ov)
}

func (s *Service) CancelOccurrence(ctx context.Context, taskID int64, date time.Time) error {
	return s.UpsertOccurrenceOverride(ctx, taskID, date, OverrideInput{IsCancelled: true})
}

func (s *Service) ForkSeries(ctx context.Context, taskID int64, fromDate time.Time, in UpdateInput, rule *RepeatInput) (*taskdomain.Task, error) {
	if taskID <= 0 {
		return nil, fmt.Errorf("%w: id must be positive", ErrInvalidInput)
	}
	normalized, err := validateUpdateInput(in)
	if err != nil {
		return nil, err
	}

	ser, err := s.repo.GetSeriesByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if ser.Rule == nil {
		return nil, fmt.Errorf("%w: task is not recurring", ErrInvalidInput)
	}

	now := s.now()
	newTask := &taskdomain.Task{
		Title:       normalized.Title,
		Description: normalized.Description,
		Status:      normalized.Status,
		ScheduledAt: normalized.ScheduledAt,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	var newRule *taskdomain.RepeatRule
	if rule == nil {
		copyRule := *ser.Rule
		copyRule.ID = 0
		copyRule.TaskID = 0
		copyRule.StartDate = truncateToDay(fromDate)
		copyRule.EndDate = nil
		newRule = &copyRule
	} else {
		built, err := buildRule(rule, normalized.ScheduledAt)
		if err != nil {
			return nil, err
		}
		built.StartDate = truncateToDay(fromDate)
		newRule = built
	}

	created, _, err := s.repo.ForkSeries(ctx, taskID, truncateToDay(fromDate), newTask, newRule)
	if err != nil {
		return nil, err
	}
	return created, nil
}

func buildRule(in *RepeatInput, scheduledAt *time.Time) (*taskdomain.RepeatRule, error) {
	start := in.StartDate
	if start.IsZero() && scheduledAt != nil {
		start = *scheduledAt
	}
	rule := &taskdomain.RepeatRule{
		Type:          in.Type,
		StartDate:     truncateToDay(start),
		EndDate:       in.EndDate,
		IntervalDays:  in.IntervalDays,
		Weekdays:      in.Weekdays,
		MonthDays:     in.MonthDays,
		SpecificDates: in.SpecificDates,
	}
	if rule.EndDate != nil {
		end := truncateToDay(*rule.EndDate)
		rule.EndDate = &end
	}
	if err := rule.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	return rule, nil
}

func validateCreateInput(input CreateInput) (CreateInput, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)

	if input.Title == "" {
		return CreateInput{}, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}

	if input.Status == "" {
		input.Status = taskdomain.StatusNew
	}

	if !input.Status.Valid() {
		return CreateInput{}, fmt.Errorf("%w: invalid status", ErrInvalidInput)
	}

	return input, nil
}

func validateUpdateInput(input UpdateInput) (UpdateInput, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)

	if input.Title == "" {
		return UpdateInput{}, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}

	if !input.Status.Valid() {
		return UpdateInput{}, fmt.Errorf("%w: invalid status", ErrInvalidInput)
	}

	return input, nil
}

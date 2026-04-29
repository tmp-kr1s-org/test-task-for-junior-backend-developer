package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	taskdomain "example.com/taskservice/internal/domain/task"
	taskusecase "example.com/taskservice/internal/usecase/task"
)

func (r *Repository) CreateWithRepeat(ctx context.Context, task *taskdomain.Task, rule *taskdomain.RepeatRule) (*taskdomain.Task, *taskdomain.RepeatRule, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)

	created, err := insertTaskTx(ctx, tx, task)
	if err != nil {
		return nil, nil, err
	}

	rule.TaskID = created.ID
	createdRule, err := insertRuleTx(ctx, tx, rule)
	if err != nil {
		return nil, nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	return created, createdRule, nil
}

func (r *Repository) GetSeriesByTaskID(ctx context.Context, taskID int64) (*taskusecase.Series, error) {
	const query = `
		SELECT
			t.id, t.title, t.description, t.status, t.scheduled_at, t.created_at, t.updated_at,
			tr.id, tr.task_id, tr.rule_type, tr.start_date, tr.end_date,
			tr.interval_days, tr.weekdays, tr.month_days, tr.specific_dates
		FROM tasks t
		LEFT JOIN task_repeats tr ON tr.task_id = t.id
		WHERE t.id = $1
	`
	row := r.pool.QueryRow(ctx, query, taskID)
	ser, err := scanSeries(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, taskdomain.ErrNotFound
		}
		return nil, err
	}
	return ser, nil
}

func (r *Repository) ListSeries(ctx context.Context, from, to *time.Time) ([]taskusecase.Series, error) {
	const baseQuery = `
		SELECT
			t.id, t.title, t.description, t.status, t.scheduled_at, t.created_at, t.updated_at,
			tr.id, tr.task_id, tr.rule_type, tr.start_date, tr.end_date,
			tr.interval_days, tr.weekdays, tr.month_days, tr.specific_dates
		FROM tasks t
		LEFT JOIN task_repeats tr ON tr.task_id = t.id
	`
	const filterQuery = baseQuery + `
		WHERE
			(tr.id IS NULL AND t.scheduled_at IS NOT NULL
			 AND t.scheduled_at::date BETWEEN $1::date AND $2::date)
			OR
			(tr.id IS NOT NULL
			 AND tr.start_date <= $2::date
			 AND (tr.end_date IS NULL OR tr.end_date >= $1::date))
		ORDER BY t.scheduled_at ASC NULLS LAST, t.id ASC
	`
	const allQuery = baseQuery + ` ORDER BY t.scheduled_at ASC NULLS LAST, t.id ASC`

	var (
		rows pgx.Rows
		err  error
	)
	if from != nil && to != nil {
		rows, err = r.pool.Query(ctx, filterQuery, *from, *to)
	} else {
		rows, err = r.pool.Query(ctx, allQuery)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]taskusecase.Series, 0)
	for rows.Next() {
		ser, err := scanSeries(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *ser)
	}
	return out, rows.Err()
}

func (r *Repository) ForkSeries(ctx context.Context, taskID int64, fromDate time.Time, newTask *taskdomain.Task, newRule *taskdomain.RepeatRule) (*taskdomain.Task, *taskdomain.RepeatRule, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)

	end := fromDate.AddDate(0, 0, -1)
	tag, err := tx.Exec(ctx,
		`UPDATE task_repeats SET end_date = $1 WHERE task_id = $2`,
		end, taskID,
	)
	if err != nil {
		return nil, nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, nil, taskdomain.ErrNotFound
	}

	created, err := insertTaskTx(ctx, tx, newTask)
	if err != nil {
		return nil, nil, err
	}
	newRule.TaskID = created.ID
	createdRule, err := insertRuleTx(ctx, tx, newRule)
	if err != nil {
		return nil, nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	return created, createdRule, nil
}

func insertTaskTx(ctx context.Context, tx pgx.Tx, task *taskdomain.Task) (*taskdomain.Task, error) {
	const q = `
		INSERT INTO tasks (title, description, status, scheduled_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, title, description, status, scheduled_at, created_at, updated_at
	`
	row := tx.QueryRow(ctx, q, task.Title, task.Description, task.Status, task.ScheduledAt, task.CreatedAt, task.UpdatedAt)
	return scanTask(row)
}

func insertRuleTx(ctx context.Context, tx pgx.Tx, rule *taskdomain.RepeatRule) (*taskdomain.RepeatRule, error) {
	const q = `
		INSERT INTO task_repeats (
			task_id, rule_type, start_date, end_date,
			interval_days, weekdays, month_days, specific_dates
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id, task_id, rule_type, start_date, end_date,
			interval_days, weekdays, month_days, specific_dates
	`
	row := tx.QueryRow(ctx, q,
		rule.TaskID, string(rule.Type), rule.StartDate, rule.EndDate,
		rule.IntervalDays, rule.Weekdays, rule.MonthDays, rule.SpecificDates,
	)
	out, err := scanRule(row)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func scanRule(scanner taskScanner) (*taskdomain.RepeatRule, error) {
	var (
		rule     taskdomain.RepeatRule
		ruleType string
	)
	if err := scanner.Scan(
		&rule.ID, &rule.TaskID, &ruleType, &rule.StartDate, &rule.EndDate,
		&rule.IntervalDays, &rule.Weekdays, &rule.MonthDays, &rule.SpecificDates,
	); err != nil {
		return nil, err
	}
	rule.Type = taskdomain.RuleType(ruleType)
	return &rule, nil
}

func scanSeries(scanner taskScanner) (*taskusecase.Series, error) {
	var (
		task        taskdomain.Task
		status      string
		scheduledAt *time.Time

		ruleID            *int64
		ruleTaskID        *int64
		ruleType          *string
		ruleStartDate     *time.Time
		ruleEndDate       *time.Time
		ruleIntervalDays  *int
		ruleWeekdays      []int32
		ruleMonthDays     []int32
		ruleSpecificDates []time.Time
	)
	if err := scanner.Scan(
		&task.ID, &task.Title, &task.Description, &status, &scheduledAt, &task.CreatedAt, &task.UpdatedAt,
		&ruleID, &ruleTaskID, &ruleType, &ruleStartDate, &ruleEndDate,
		&ruleIntervalDays, &ruleWeekdays, &ruleMonthDays, &ruleSpecificDates,
	); err != nil {
		return nil, err
	}
	task.Status = taskdomain.Status(status)
	task.ScheduledAt = scheduledAt

	ser := &taskusecase.Series{Task: task}
	if ruleID != nil {
		rule := &taskdomain.RepeatRule{
			ID:            *ruleID,
			TaskID:        *ruleTaskID,
			Type:          taskdomain.RuleType(*ruleType),
			StartDate:     *ruleStartDate,
			EndDate:       ruleEndDate,
			IntervalDays:  ruleIntervalDays,
			SpecificDates: ruleSpecificDates,
		}
		rule.Weekdays = int32sToInts(ruleWeekdays)
		rule.MonthDays = int32sToInts(ruleMonthDays)
		ser.Rule = rule
	}
	return ser, nil
}

func int32sToInts(in []int32) []int {
	if in == nil {
		return nil
	}
	out := make([]int, len(in))
	for i, v := range in {
		out[i] = int(v)
	}
	return out
}

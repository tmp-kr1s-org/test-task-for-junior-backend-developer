package postgres

import (
	"context"
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
	taskusecase "example.com/taskservice/internal/usecase/task"
)

func (r *Repository) UpsertOverride(ctx context.Context, ov taskdomain.Override) error {
	const q = `
		INSERT INTO task_repeats_updates (
			repeat_id, occurrence_date, title, description, status, scheduled_at, is_cancelled
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (repeat_id, occurrence_date) DO UPDATE SET
			title        = COALESCE(EXCLUDED.title,        task_repeats_updates.title),
			description  = COALESCE(EXCLUDED.description,  task_repeats_updates.description),
			status       = COALESCE(EXCLUDED.status,       task_repeats_updates.status),
			scheduled_at = COALESCE(EXCLUDED.scheduled_at, task_repeats_updates.scheduled_at),
			is_cancelled = EXCLUDED.is_cancelled
	`
	var statusArg any
	if ov.Status != nil {
		statusArg = string(*ov.Status)
	}
	_, err := r.pool.Exec(ctx, q,
		ov.RepeatID, ov.Date, ov.Title, ov.Description, statusArg, ov.ScheduledAt, ov.IsCancelled,
	)
	return err
}

func (r *Repository) LoadOverrides(ctx context.Context, repeatIDs []int64, from, to time.Time) (map[int64]map[string]taskdomain.Override, error) {
	out := map[int64]map[string]taskdomain.Override{}
	if len(repeatIDs) == 0 {
		return out, nil
	}
	const q = `
		SELECT repeat_id, occurrence_date, title, description, status, scheduled_at, is_cancelled
		FROM task_repeats_updates
		WHERE repeat_id = ANY($1)
		  AND occurrence_date BETWEEN $2::date AND $3::date
	`
	rows, err := r.pool.Query(ctx, q, repeatIDs, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			repeatID int64
			date     time.Time
			title    *string
			desc     *string
			status   *string
			schedAt  *time.Time
			cancel   bool
		)
		if err := rows.Scan(&repeatID, &date, &title, &desc, &status, &schedAt, &cancel); err != nil {
			return nil, err
		}
		ov := taskdomain.Override{
			RepeatID:    repeatID,
			Date:        date,
			Title:       title,
			Description: desc,
			ScheduledAt: schedAt,
			IsCancelled: cancel,
		}
		if status != nil {
			s := taskdomain.Status(*status)
			ov.Status = &s
		}
		bucket, ok := out[repeatID]
		if !ok {
			bucket = map[string]taskdomain.Override{}
			out[repeatID] = bucket
		}
		bucket[taskusecase.DateKey(date)] = ov
	}
	return out, rows.Err()
}

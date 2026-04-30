ALTER TABLE tasks ADD COLUMN IF NOT EXISTS scheduled_at TIMESTAMPTZ NULL;

CREATE TABLE IF NOT EXISTS task_repeats (
	id BIGSERIAL PRIMARY KEY,
	task_id BIGINT NOT NULL UNIQUE REFERENCES tasks(id) ON DELETE CASCADE,
	rule_type TEXT NOT NULL CHECK (rule_type IN
		('daily','weekly','monthly','specific_dates','even_days','odd_days')),
	start_date DATE NOT NULL,
	end_date DATE NULL,
	interval_days INT NULL,
	weekdays INT[] NULL,
	month_days INT[] NULL,
	specific_dates DATE[] NULL,
	CHECK (end_date IS NULL OR end_date >= start_date)
);

CREATE INDEX IF NOT EXISTS idx_task_repeats_range ON task_repeats (start_date, end_date);

CREATE TABLE IF NOT EXISTS task_repeats_updates (
	repeat_id BIGINT NOT NULL REFERENCES task_repeats(id) ON DELETE CASCADE,
	occurrence_date DATE NOT NULL,
	title TEXT NULL,
	description TEXT NULL,
	status TEXT NULL,
	scheduled_at TIMESTAMPTZ NULL,
	is_cancelled BOOLEAN NOT NULL DEFAULT FALSE,
	PRIMARY KEY (repeat_id, occurrence_date)
);

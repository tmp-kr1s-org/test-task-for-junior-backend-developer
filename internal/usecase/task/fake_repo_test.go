package task

import (
	"context"
	"errors"
	"sync"
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
)

// fakeRepo is an in-memory Repository implementation for tests.
type fakeRepo struct {
	mu     sync.Mutex
	nextID int64
	items  map[int64]taskdomain.Task

	nextRepeatID int64
	rules        map[int64]taskdomain.RepeatRule       // key: repeat_id
	rulesByTask  map[int64]int64                       // task_id -> repeat_id
	overrides    map[int64]map[string]taskdomain.Override // repeat_id -> dateKey -> override

	createErr error
	getErr    error
	updateErr error
	deleteErr error
	listErr   error

	lastCreated *taskdomain.Task
	lastUpdated *taskdomain.Task
	deletedIDs  []int64
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		items:       map[int64]taskdomain.Task{},
		rules:       map[int64]taskdomain.RepeatRule{},
		rulesByTask: map[int64]int64{},
		overrides:   map[int64]map[string]taskdomain.Override{},
	}
}

func (r *fakeRepo) Create(_ context.Context, t *taskdomain.Task) (*taskdomain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createErr != nil {
		return nil, r.createErr
	}
	r.nextID++
	cp := *t
	cp.ID = r.nextID
	r.items[cp.ID] = cp
	r.lastCreated = &cp
	out := cp
	return &out, nil
}

func (r *fakeRepo) GetByID(_ context.Context, id int64) (*taskdomain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.getErr != nil {
		return nil, r.getErr
	}
	v, ok := r.items[id]
	if !ok {
		return nil, taskdomain.ErrNotFound
	}
	out := v
	return &out, nil
}

func (r *fakeRepo) Update(_ context.Context, t *taskdomain.Task) (*taskdomain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.updateErr != nil {
		return nil, r.updateErr
	}
	existing, ok := r.items[t.ID]
	if !ok {
		return nil, taskdomain.ErrNotFound
	}
	updated := *t
	updated.CreatedAt = existing.CreatedAt
	r.items[t.ID] = updated
	r.lastUpdated = &updated
	out := updated
	return &out, nil
}

func (r *fakeRepo) Delete(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.deleteErr != nil {
		return r.deleteErr
	}
	if _, ok := r.items[id]; !ok {
		return taskdomain.ErrNotFound
	}
	delete(r.items, id)
	r.deletedIDs = append(r.deletedIDs, id)
	return nil
}

func (r *fakeRepo) List(_ context.Context) ([]taskdomain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.listErr != nil {
		return nil, r.listErr
	}
	out := make([]taskdomain.Task, 0, len(r.items))
	for _, v := range r.items {
		out = append(out, v)
	}
	return out, nil
}

func (r *fakeRepo) CreateWithRepeat(ctx context.Context, t *taskdomain.Task, rule *taskdomain.RepeatRule) (*taskdomain.Task, *taskdomain.RepeatRule, error) {
	created, err := r.Create(ctx, t)
	if err != nil {
		return nil, nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextRepeatID++
	cp := *rule
	cp.ID = r.nextRepeatID
	cp.TaskID = created.ID
	r.rules[cp.ID] = cp
	r.rulesByTask[created.ID] = cp.ID
	out := cp
	return created, &out, nil
}

func (r *fakeRepo) GetSeriesByTaskID(_ context.Context, taskID int64) (*Series, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.items[taskID]
	if !ok {
		return nil, taskdomain.ErrNotFound
	}
	ser := &Series{Task: t}
	if rid, has := r.rulesByTask[taskID]; has {
		rule := r.rules[rid]
		ser.Rule = &rule
	}
	return ser, nil
}

func (r *fakeRepo) ListSeries(_ context.Context, from, to *time.Time) ([]Series, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Series, 0, len(r.items))
	for _, t := range r.items {
		ser := Series{Task: t}
		if rid, has := r.rulesByTask[t.ID]; has {
			rule := r.rules[rid]
			ser.Rule = &rule
		}
		out = append(out, ser)
	}
	_ = from
	_ = to
	return out, nil
}

func (r *fakeRepo) LoadOverrides(_ context.Context, repeatIDs []int64, from, to time.Time) (map[int64]map[string]taskdomain.Override, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[int64]map[string]taskdomain.Override{}
	for _, rid := range repeatIDs {
		entries := r.overrides[rid]
		if len(entries) == 0 {
			continue
		}
		bucket := map[string]taskdomain.Override{}
		for k, v := range entries {
			d := v.Date
			if (d.Equal(from) || d.After(from)) && (d.Equal(to) || d.Before(to)) {
				bucket[k] = v
			}
		}
		if len(bucket) > 0 {
			out[rid] = bucket
		}
	}
	return out, nil
}

func (r *fakeRepo) UpsertOverride(_ context.Context, ov taskdomain.Override) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.rules[ov.RepeatID]; !ok {
		return taskdomain.ErrNotFound
	}
	bucket, ok := r.overrides[ov.RepeatID]
	if !ok {
		bucket = map[string]taskdomain.Override{}
		r.overrides[ov.RepeatID] = bucket
	}
	bucket[DateKey(ov.Date)] = ov
	return nil
}

func (r *fakeRepo) ForkSeries(ctx context.Context, taskID int64, fromDate time.Time, newTask *taskdomain.Task, newRule *taskdomain.RepeatRule) (*taskdomain.Task, *taskdomain.RepeatRule, error) {
	r.mu.Lock()
	rid, ok := r.rulesByTask[taskID]
	if !ok {
		r.mu.Unlock()
		return nil, nil, taskdomain.ErrNotFound
	}
	old := r.rules[rid]
	end := fromDate.AddDate(0, 0, -1)
	old.EndDate = &end
	r.rules[rid] = old
	r.mu.Unlock()

	return r.CreateWithRepeat(ctx, newTask, newRule)
}

// errBoom is a sentinel for "infrastructure failed".
var errBoom = errors.New("boom")

package task

import (
	"context"
	"errors"
	"sync"

	taskdomain "example.com/taskservice/internal/domain/task"
)

// fakeRepo is an in-memory Repository implementation for tests.
type fakeRepo struct {
	mu     sync.Mutex
	nextID int64
	items  map[int64]taskdomain.Task

	// hooks let tests force errors from any method.
	createErr  error
	getErr     error
	updateErr  error
	deleteErr  error
	listErr    error

	// captured args for assertions.
	lastCreated *taskdomain.Task
	lastUpdated *taskdomain.Task
	deletedIDs  []int64
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{items: map[int64]taskdomain.Task{}}
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
	// preserve created_at as a real repo would.
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

// errBoom is a sentinel for "infrastructure failed".
var errBoom = errors.New("boom")

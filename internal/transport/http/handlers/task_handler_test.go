package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
	transporthttp "example.com/taskservice/internal/transport/http"
	swaggerdocs "example.com/taskservice/internal/transport/http/docs"
	"example.com/taskservice/internal/transport/http/handlers"
	taskusecase "example.com/taskservice/internal/usecase/task"
)

// stubUsecase is a hand-rolled mock of taskusecase.Usecase configured per test.
type stubUsecase struct {
	createFn       func(ctx context.Context, in taskusecase.CreateInput) (*taskdomain.Task, error)
	getFn          func(ctx context.Context, id int64) (*taskdomain.Task, error)
	getSeriesFn    func(ctx context.Context, id int64) (*taskusecase.Series, error)
	updateFn       func(ctx context.Context, id int64, in taskusecase.UpdateInput) (*taskdomain.Task, error)
	deleteFn func(ctx context.Context, id int64) error
	listFn   func(ctx context.Context) ([]taskdomain.Task, error)

	listOccurrencesFn func(ctx context.Context, from, to time.Time) ([]taskdomain.Occurrence, error)
	upsertOverrideFn  func(ctx context.Context, taskID int64, date time.Time, in taskusecase.OverrideInput) error
	cancelFn          func(ctx context.Context, taskID int64, date time.Time) error
	forkFn            func(ctx context.Context, taskID int64, fromDate time.Time, in taskusecase.UpdateInput, rule *taskusecase.RepeatInput) (*taskdomain.Task, error)

	createCalls atomic.Int64
	getCalls    atomic.Int64
	updateCalls atomic.Int64
	deleteCalls atomic.Int64
	listCalls   atomic.Int64
}

func (s *stubUsecase) ListOccurrences(ctx context.Context, from, to time.Time) ([]taskdomain.Occurrence, error) {
	if s.listOccurrencesFn == nil {
		return nil, errors.New("listOccurrencesFn not set")
	}
	return s.listOccurrencesFn(ctx, from, to)
}

func (s *stubUsecase) UpsertOccurrenceOverride(ctx context.Context, taskID int64, date time.Time, in taskusecase.OverrideInput) error {
	if s.upsertOverrideFn == nil {
		return errors.New("upsertOverrideFn not set")
	}
	return s.upsertOverrideFn(ctx, taskID, date, in)
}

func (s *stubUsecase) CancelOccurrence(ctx context.Context, taskID int64, date time.Time) error {
	if s.cancelFn == nil {
		return errors.New("cancelFn not set")
	}
	return s.cancelFn(ctx, taskID, date)
}

func (s *stubUsecase) ForkSeries(ctx context.Context, taskID int64, fromDate time.Time, in taskusecase.UpdateInput, rule *taskusecase.RepeatInput) (*taskdomain.Task, error) {
	if s.forkFn == nil {
		return nil, errors.New("forkFn not set")
	}
	return s.forkFn(ctx, taskID, fromDate, in, rule)
}

func (s *stubUsecase) Create(ctx context.Context, in taskusecase.CreateInput) (*taskdomain.Task, error) {
	s.createCalls.Add(1)
	if s.createFn == nil {
		return nil, errors.New("createFn not set")
	}
	return s.createFn(ctx, in)
}

func (s *stubUsecase) GetByID(ctx context.Context, id int64) (*taskdomain.Task, error) {
	s.getCalls.Add(1)
	if s.getFn == nil {
		return nil, errors.New("getFn not set")
	}
	return s.getFn(ctx, id)
}

func (s *stubUsecase) GetSeriesByID(ctx context.Context, id int64) (*taskusecase.Series, error) {
	s.getCalls.Add(1)
	if s.getSeriesFn != nil {
		return s.getSeriesFn(ctx, id)
	}
	if s.getFn != nil {
		t, err := s.getFn(ctx, id)
		if err != nil {
			return nil, err
		}
		return &taskusecase.Series{Task: *t}, nil
	}
	return nil, errors.New("getSeriesFn not set")
}

func (s *stubUsecase) Update(ctx context.Context, id int64, in taskusecase.UpdateInput) (*taskdomain.Task, error) {
	s.updateCalls.Add(1)
	if s.updateFn == nil {
		return nil, errors.New("updateFn not set")
	}
	return s.updateFn(ctx, id, in)
}

func (s *stubUsecase) Delete(ctx context.Context, id int64) error {
	s.deleteCalls.Add(1)
	if s.deleteFn == nil {
		return errors.New("deleteFn not set")
	}
	return s.deleteFn(ctx, id)
}

func (s *stubUsecase) List(ctx context.Context) ([]taskdomain.Task, error) {
	s.listCalls.Add(1)
	if s.listFn == nil {
		return nil, errors.New("listFn not set")
	}
	return s.listFn(ctx)
}

// newServer builds the real router around a stub usecase.
func newServer(uc taskusecase.Usecase) http.Handler {
	taskHandler := handlers.NewTaskHandler(uc)
	docsHandler := swaggerdocs.NewHandler()
	return transporthttp.NewRouter(taskHandler, docsHandler)
}

func doJSON(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		switch v := body.(type) {
		case string:
			reader = strings.NewReader(v)
		case []byte:
			reader = bytes.NewReader(v)
		default:
			buf, err := json.Marshal(v)
			if err != nil {
				t.Fatalf("marshal body: %v", err)
			}
			reader = bytes.NewReader(buf)
		}
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
}

func sampleTask() *taskdomain.Task {
	return &taskdomain.Task{
		ID:          1,
		Title:       "buy milk",
		Description: "2L",
		Status:      taskdomain.StatusNew,
		CreatedAt:   time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC),
	}
}

// ---------- POST /api/v1/tasks ----------

func TestCreate_HappyPath(t *testing.T) {
	uc := &stubUsecase{
		createFn: func(_ context.Context, in taskusecase.CreateInput) (*taskdomain.Task, error) {
			if in.Title != "buy milk" || in.Description != "2L" || in.Status != taskdomain.StatusInProgress {
				t.Errorf("usecase received unexpected input: %+v", in)
			}
			return &taskdomain.Task{
				ID: 42, Title: in.Title, Description: in.Description, Status: in.Status,
			}, nil
		},
	}
	srv := newServer(uc)

	rec := doJSON(t, srv, http.MethodPost, "/api/v1/tasks", map[string]any{
		"title":       "buy milk",
		"description": "2L",
		"status":      "in_progress",
	})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status: want 201, got %d (body=%s)", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type: %q", ct)
	}
	var got map[string]any
	decodeBody(t, rec, &got)
	if got["id"].(float64) != 42 {
		t.Errorf("id mismatch: %v", got["id"])
	}
	if got["status"] != "in_progress" {
		t.Errorf("status mismatch: %v", got["status"])
	}
}

func TestCreate_BadRequests(t *testing.T) {
	cases := []struct {
		name string
		body any
	}{
		{"invalid JSON", "{not-json"},
		{"empty body", ""},
		{"unknown field", map[string]any{"title": "x", "extra": "nope"}},
		{"wrong type for status", map[string]any{"title": "x", "status": 5}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			uc := &stubUsecase{
				createFn: func(context.Context, taskusecase.CreateInput) (*taskdomain.Task, error) {
					t.Fatalf("usecase should not be called when JSON decoding fails")
					return nil, nil
				},
			}
			srv := newServer(uc)
			rec := doJSON(t, srv, http.MethodPost, "/api/v1/tasks", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("want 400, got %d (body=%s)", rec.Code, rec.Body)
			}
			var env map[string]string
			decodeBody(t, rec, &env)
			if env["error"] == "" {
				t.Errorf("expected error envelope, got %v", env)
			}
		})
	}
}

func TestCreate_UsecaseValidationError_Maps400(t *testing.T) {
	uc := &stubUsecase{
		createFn: func(context.Context, taskusecase.CreateInput) (*taskdomain.Task, error) {
			return nil, fmt.Errorf("%w: title is required", taskusecase.ErrInvalidInput)
		},
	}
	srv := newServer(uc)
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/tasks", map[string]any{"title": ""})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestCreate_UsecaseInternalError_Maps500(t *testing.T) {
	uc := &stubUsecase{
		createFn: func(context.Context, taskusecase.CreateInput) (*taskdomain.Task, error) {
			return nil, errors.New("db down")
		},
	}
	srv := newServer(uc)
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/tasks", map[string]any{"title": "x"})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rec.Code)
	}
}

// ---------- GET /api/v1/tasks/{id} ----------

func TestGetByID_HappyPath(t *testing.T) {
	want := sampleTask()
	uc := &stubUsecase{
		getFn: func(_ context.Context, id int64) (*taskdomain.Task, error) {
			if id != 1 {
				t.Errorf("expected id=1, got %d", id)
			}
			return want, nil
		},
	}
	srv := newServer(uc)

	rec := doJSON(t, srv, http.MethodGet, "/api/v1/tasks/1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var got taskdomain.Task
	decodeBody(t, rec, &got)
	if got.ID != want.ID || got.Title != want.Title || got.Status != want.Status {
		t.Errorf("payload mismatch: %+v", got)
	}
}

func TestGetByID_NotFound(t *testing.T) {
	uc := &stubUsecase{
		getFn: func(context.Context, int64) (*taskdomain.Task, error) {
			return nil, taskdomain.ErrNotFound
		},
	}
	srv := newServer(uc)
	rec := doJSON(t, srv, http.MethodGet, "/api/v1/tasks/777", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestGetByID_NonNumericIDDoesNotMatchRoute(t *testing.T) {
	uc := &stubUsecase{
		getFn: func(context.Context, int64) (*taskdomain.Task, error) {
			t.Fatalf("usecase must not be called when id is non-numeric")
			return nil, nil
		},
	}
	srv := newServer(uc)
	rec := doJSON(t, srv, http.MethodGet, "/api/v1/tasks/abc", nil)
	// gorilla/mux returns 404 because the {id:[0-9]+} constraint blocks the route.
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 from router, got %d", rec.Code)
	}
}

func TestGetByID_NegativeIDDoesNotMatchRoute(t *testing.T) {
	uc := &stubUsecase{
		getFn: func(context.Context, int64) (*taskdomain.Task, error) {
			t.Fatalf("usecase must not be called for negative id")
			return nil, nil
		},
	}
	srv := newServer(uc)
	rec := doJSON(t, srv, http.MethodGet, "/api/v1/tasks/-1", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestGetByID_InternalError(t *testing.T) {
	uc := &stubUsecase{
		getFn: func(context.Context, int64) (*taskdomain.Task, error) {
			return nil, errors.New("db down")
		},
	}
	srv := newServer(uc)
	rec := doJSON(t, srv, http.MethodGet, "/api/v1/tasks/1", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rec.Code)
	}
}

// ---------- PUT /api/v1/tasks/{id} ----------

func TestUpdate_HappyPath(t *testing.T) {
	uc := &stubUsecase{
		updateFn: func(_ context.Context, id int64, in taskusecase.UpdateInput) (*taskdomain.Task, error) {
			if id != 7 {
				t.Errorf("expected id=7, got %d", id)
			}
			if in.Title != "new" || in.Status != taskdomain.StatusDone {
				t.Errorf("input mismatch: %+v", in)
			}
			return &taskdomain.Task{ID: id, Title: in.Title, Description: in.Description, Status: in.Status}, nil
		},
	}
	srv := newServer(uc)
	rec := doJSON(t, srv, http.MethodPut, "/api/v1/tasks/7", map[string]any{
		"title":       "new",
		"description": "d",
		"status":      "done",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", rec.Code, rec.Body)
	}
}

func TestUpdate_NotFound(t *testing.T) {
	uc := &stubUsecase{
		updateFn: func(context.Context, int64, taskusecase.UpdateInput) (*taskdomain.Task, error) {
			return nil, taskdomain.ErrNotFound
		},
	}
	srv := newServer(uc)
	rec := doJSON(t, srv, http.MethodPut, "/api/v1/tasks/9999", map[string]any{
		"title": "x", "status": "new",
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestUpdate_ValidationError(t *testing.T) {
	uc := &stubUsecase{
		updateFn: func(context.Context, int64, taskusecase.UpdateInput) (*taskdomain.Task, error) {
			return nil, fmt.Errorf("%w: invalid status", taskusecase.ErrInvalidInput)
		},
	}
	srv := newServer(uc)
	rec := doJSON(t, srv, http.MethodPut, "/api/v1/tasks/1", map[string]any{
		"title": "x", "status": "weird",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestUpdate_BadJSONRejectedBeforeUsecase(t *testing.T) {
	uc := &stubUsecase{
		updateFn: func(context.Context, int64, taskusecase.UpdateInput) (*taskdomain.Task, error) {
			t.Fatalf("usecase must not be called on bad JSON")
			return nil, nil
		},
	}
	srv := newServer(uc)
	rec := doJSON(t, srv, http.MethodPut, "/api/v1/tasks/1", "not json")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

// ---------- DELETE /api/v1/tasks/{id} ----------

func TestDelete_HappyPath(t *testing.T) {
	uc := &stubUsecase{
		deleteFn: func(_ context.Context, id int64) error {
			if id != 5 {
				t.Errorf("expected id=5, got %d", id)
			}
			return nil
		},
	}
	srv := newServer(uc)
	rec := doJSON(t, srv, http.MethodDelete, "/api/v1/tasks/5", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("204 must have empty body, got %q", rec.Body.String())
	}
}

func TestDelete_NotFound(t *testing.T) {
	uc := &stubUsecase{
		deleteFn: func(context.Context, int64) error { return taskdomain.ErrNotFound },
	}
	srv := newServer(uc)
	rec := doJSON(t, srv, http.MethodDelete, "/api/v1/tasks/5", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestDelete_InternalError(t *testing.T) {
	uc := &stubUsecase{
		deleteFn: func(context.Context, int64) error { return errors.New("db down") },
	}
	srv := newServer(uc)
	rec := doJSON(t, srv, http.MethodDelete, "/api/v1/tasks/5", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rec.Code)
	}
}

// ---------- GET /api/v1/tasks ----------

func TestList_HappyPath(t *testing.T) {
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	uc := &stubUsecase{
		listFn: func(context.Context) ([]taskdomain.Task, error) {
			return []taskdomain.Task{
				{ID: 1, Title: "a", Status: taskdomain.StatusNew, CreatedAt: now, UpdatedAt: now},
				{ID: 2, Title: "b", Status: taskdomain.StatusDone, CreatedAt: now, UpdatedAt: now},
			}, nil
		},
	}
	srv := newServer(uc)
	rec := doJSON(t, srv, http.MethodGet, "/api/v1/tasks", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var got []taskdomain.Task
	decodeBody(t, rec, &got)
	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 2 {
		t.Errorf("payload mismatch: %+v", got)
	}
}

func TestList_EmptyReturnsEmptyArrayNotNull(t *testing.T) {
	uc := &stubUsecase{
		listFn: func(context.Context) ([]taskdomain.Task, error) {
			return nil, nil
		},
	}
	srv := newServer(uc)
	rec := doJSON(t, srv, http.MethodGet, "/api/v1/tasks", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := strings.TrimSpace(rec.Body.String())
	if body != "[]" {
		t.Errorf("empty list must serialize as [], got %q", body)
	}
}

func TestList_InternalError(t *testing.T) {
	uc := &stubUsecase{
		listFn: func(context.Context) ([]taskdomain.Task, error) {
			return nil, errors.New("db down")
		},
	}
	srv := newServer(uc)
	rec := doJSON(t, srv, http.MethodGet, "/api/v1/tasks", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rec.Code)
	}
}

// ---------- routing-level guarantees ----------

func TestMethodNotAllowed(t *testing.T) {
	uc := &stubUsecase{}
	srv := newServer(uc)
	// PATCH on a route that only knows GET/POST/PUT/DELETE — gorilla/mux returns 405.
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/1", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", rec.Code)
	}
}

func TestUnknownPath(t *testing.T) {
	uc := &stubUsecase{}
	srv := newServer(uc)
	rec := doJSON(t, srv, http.MethodGet, "/api/v1/no-such-thing", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

// large id near int64 boundary should still hit the usecase (router accepts any [0-9]+).
func TestGetByID_VeryLargeID(t *testing.T) {
	uc := &stubUsecase{
		getFn: func(_ context.Context, id int64) (*taskdomain.Task, error) {
			if id <= 0 {
				t.Errorf("expected positive id, got %d", id)
			}
			return nil, taskdomain.ErrNotFound
		},
	}
	srv := newServer(uc)
	rec := doJSON(t, srv, http.MethodGet, "/api/v1/tasks/9223372036854775807", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

// id that overflows int64 ⇒ ParseInt fails ⇒ handler returns 400.
func TestGetByID_IDOverflows(t *testing.T) {
	uc := &stubUsecase{
		getFn: func(context.Context, int64) (*taskdomain.Task, error) {
			t.Fatalf("usecase must not be called when id parsing fails")
			return nil, nil
		},
	}
	srv := newServer(uc)
	rec := doJSON(t, srv, http.MethodGet, "/api/v1/tasks/99999999999999999999", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d (body=%s)", rec.Code, rec.Body)
	}
}

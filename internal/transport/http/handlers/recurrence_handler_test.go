package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
	taskusecase "example.com/taskservice/internal/usecase/task"
)

func TestList_RangeReturnsOccurrences(t *testing.T) {
	uc := &stubUsecase{
		listOccurrencesFn: func(_ context.Context, from, to time.Time) ([]taskdomain.Occurrence, error) {
			if from.Format("2006-01-02") != "2026-04-25" || to.Format("2006-01-02") != "2026-04-29" {
				t.Errorf("range parsing wrong: %v..%v", from, to)
			}
			return []taskdomain.Occurrence{
				{Task: taskdomain.Task{ID: 1, Title: "t", Status: taskdomain.StatusNew, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}, Date: time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)},
			}, nil
		},
	}
	srv := newServer(uc)
	rec := doJSON(t, srv, http.MethodGet, "/api/v1/tasks?from=2026-04-25&to=2026-04-29", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d (body=%s)", rec.Code, rec.Body)
	}
	var arr []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &arr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(arr) != 1 {
		t.Fatalf("want 1 occurrence, got %d", len(arr))
	}
	if arr[0]["date"] != "2026-04-27" {
		t.Errorf("date missing/wrong: %v", arr[0]["date"])
	}
}

func TestList_RangeBadDates(t *testing.T) {
	uc := &stubUsecase{}
	srv := newServer(uc)
	rec := doJSON(t, srv, http.MethodGet, "/api/v1/tasks?from=garbage&to=2026-04-29", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestUpsertOverride_HappyPath(t *testing.T) {
	called := false
	uc := &stubUsecase{
		upsertOverrideFn: func(_ context.Context, taskID int64, date time.Time, in taskusecase.OverrideInput) error {
			called = true
			if taskID != 7 || date.Format("2006-01-02") != "2026-04-29" {
				t.Errorf("args wrong: id=%d date=%v", taskID, date)
			}
			if in.Status == nil || *in.Status != taskdomain.StatusDone {
				t.Errorf("status not propagated: %+v", in)
			}
			return nil
		},
	}
	srv := newServer(uc)
	rec := doJSON(t, srv, http.MethodPut, "/api/v1/tasks/7/occurrences/2026-04-29", map[string]any{
		"status": "done",
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d (body=%s)", rec.Code, rec.Body)
	}
	if !called {
		t.Errorf("usecase not invoked")
	}
}

func TestCancelOccurrence_HappyPath(t *testing.T) {
	called := false
	uc := &stubUsecase{
		cancelFn: func(_ context.Context, taskID int64, date time.Time) error {
			called = true
			if taskID != 5 || date.Format("2006-01-02") != "2026-05-01" {
				t.Errorf("args wrong: id=%d date=%v", taskID, date)
			}
			return nil
		},
	}
	srv := newServer(uc)
	rec := doJSON(t, srv, http.MethodDelete, "/api/v1/tasks/5/occurrences/2026-05-01", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", rec.Code)
	}
	if !called {
		t.Errorf("usecase not invoked")
	}
}

func TestFork_HappyPath(t *testing.T) {
	uc := &stubUsecase{
		forkFn: func(_ context.Context, taskID int64, fromDate time.Time, in taskusecase.UpdateInput, rule *taskusecase.RepeatInput) (*taskdomain.Task, error) {
			if taskID != 3 || fromDate.Format("2006-01-02") != "2026-05-04" {
				t.Errorf("args wrong: id=%d from=%v", taskID, fromDate)
			}
			if in.Title != "new title" {
				t.Errorf("title not propagated: %q", in.Title)
			}
			return &taskdomain.Task{ID: 99, Title: in.Title, Status: in.Status, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}, nil
		},
	}
	srv := newServer(uc)
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/tasks/3/fork", map[string]any{
		"from_date": "2026-05-04",
		"title":     "new title",
		"status":    "new",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d (body=%s)", rec.Code, rec.Body)
	}
}

package task

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
)

func newServiceWithFixedNow(repo Repository, now time.Time) *Service {
	s := NewService(repo)
	s.now = func() time.Time { return now }
	return s
}

func TestService_Create(t *testing.T) {
	fixed := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)

	t.Run("happy path with explicit status", func(t *testing.T) {
		repo := newFakeRepo()
		svc := newServiceWithFixedNow(repo, fixed)

		got, err := svc.Create(context.Background(), CreateInput{
			Title:       "  buy milk  ",
			Description: "  2L  ",
			Status:      taskdomain.StatusInProgress,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID == 0 {
			t.Errorf("expected assigned ID, got 0")
		}
		if got.Title != "buy milk" {
			t.Errorf("title not trimmed: %q", got.Title)
		}
		if got.Description != "2L" {
			t.Errorf("description not trimmed: %q", got.Description)
		}
		if got.Status != taskdomain.StatusInProgress {
			t.Errorf("status mismatch: %q", got.Status)
		}
		if !got.CreatedAt.Equal(fixed) || !got.UpdatedAt.Equal(fixed) {
			t.Errorf("timestamps not set from now(): created=%v updated=%v", got.CreatedAt, got.UpdatedAt)
		}
		if !got.CreatedAt.Equal(got.UpdatedAt) {
			t.Errorf("on create, created_at must equal updated_at")
		}
	})

	t.Run("default status is new when empty", func(t *testing.T) {
		repo := newFakeRepo()
		svc := newServiceWithFixedNow(repo, fixed)

		got, err := svc.Create(context.Background(), CreateInput{Title: "x"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Status != taskdomain.StatusNew {
			t.Errorf("expected default status 'new', got %q", got.Status)
		}
	})

	t.Run("rejects empty title", func(t *testing.T) {
		repo := newFakeRepo()
		svc := newServiceWithFixedNow(repo, fixed)

		_, err := svc.Create(context.Background(), CreateInput{Title: ""})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected ErrInvalidInput, got %v", err)
		}
	})

	t.Run("rejects whitespace-only title", func(t *testing.T) {
		repo := newFakeRepo()
		svc := newServiceWithFixedNow(repo, fixed)

		_, err := svc.Create(context.Background(), CreateInput{Title: "   \t\n  "})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected ErrInvalidInput, got %v", err)
		}
	})

	t.Run("rejects unknown status", func(t *testing.T) {
		repo := newFakeRepo()
		svc := newServiceWithFixedNow(repo, fixed)

		_, err := svc.Create(context.Background(), CreateInput{Title: "x", Status: "archived"})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected ErrInvalidInput, got %v", err)
		}
	})

	t.Run("propagates repository error", func(t *testing.T) {
		repo := newFakeRepo()
		repo.createErr = errBoom
		svc := newServiceWithFixedNow(repo, fixed)

		_, err := svc.Create(context.Background(), CreateInput{Title: "x"})
		if !errors.Is(err, errBoom) {
			t.Fatalf("expected errBoom, got %v", err)
		}
	})

	t.Run("does not call repo when validation fails", func(t *testing.T) {
		repo := newFakeRepo()
		svc := newServiceWithFixedNow(repo, fixed)

		_, _ = svc.Create(context.Background(), CreateInput{Title: ""})
		if repo.lastCreated != nil {
			t.Errorf("repo.Create should not be called on invalid input")
		}
	})
}

func TestService_GetByID(t *testing.T) {
	fixed := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)

	t.Run("happy path", func(t *testing.T) {
		repo := newFakeRepo()
		svc := newServiceWithFixedNow(repo, fixed)
		created, _ := svc.Create(context.Background(), CreateInput{Title: "x"})

		got, err := svc.GetByID(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != created.ID {
			t.Errorf("id mismatch")
		}
	})

	t.Run("zero id rejected", func(t *testing.T) {
		svc := newServiceWithFixedNow(newFakeRepo(), fixed)
		_, err := svc.GetByID(context.Background(), 0)
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected ErrInvalidInput, got %v", err)
		}
	})

	t.Run("negative id rejected", func(t *testing.T) {
		svc := newServiceWithFixedNow(newFakeRepo(), fixed)
		_, err := svc.GetByID(context.Background(), -5)
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected ErrInvalidInput, got %v", err)
		}
	})

	t.Run("not found bubbles up", func(t *testing.T) {
		svc := newServiceWithFixedNow(newFakeRepo(), fixed)
		_, err := svc.GetByID(context.Background(), 999)
		if !errors.Is(err, taskdomain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestService_Update(t *testing.T) {
	created := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	updated := created.Add(2 * time.Hour)

	setup := func() (*Service, *fakeRepo, int64) {
		repo := newFakeRepo()
		svc := newServiceWithFixedNow(repo, created)
		got, err := svc.Create(context.Background(), CreateInput{Title: "orig", Status: taskdomain.StatusNew})
		if err != nil {
			t.Fatalf("setup create failed: %v", err)
		}
		svc.now = func() time.Time { return updated }
		return svc, repo, got.ID
	}

	t.Run("happy path updates fields and updated_at", func(t *testing.T) {
		svc, _, id := setup()
		got, err := svc.Update(context.Background(), id, UpdateInput{
			Title:       " new title ",
			Description: " new desc ",
			Status:      taskdomain.StatusDone,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Title != "new title" || got.Description != "new desc" {
			t.Errorf("trim/update failed: %+v", got)
		}
		if got.Status != taskdomain.StatusDone {
			t.Errorf("status not updated: %q", got.Status)
		}
		if !got.UpdatedAt.Equal(updated) {
			t.Errorf("updated_at not refreshed: %v", got.UpdatedAt)
		}
		if !got.CreatedAt.Equal(created) {
			t.Errorf("created_at must be preserved by repo, got %v", got.CreatedAt)
		}
	})

	t.Run("rejects zero id", func(t *testing.T) {
		svc, _, _ := setup()
		_, err := svc.Update(context.Background(), 0, UpdateInput{Title: "x", Status: taskdomain.StatusNew})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected ErrInvalidInput, got %v", err)
		}
	})

	t.Run("rejects empty title", func(t *testing.T) {
		svc, _, id := setup()
		_, err := svc.Update(context.Background(), id, UpdateInput{Title: "  ", Status: taskdomain.StatusNew})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected ErrInvalidInput, got %v", err)
		}
	})

	t.Run("rejects empty status (no default on update)", func(t *testing.T) {
		svc, _, id := setup()
		_, err := svc.Update(context.Background(), id, UpdateInput{Title: "x", Status: ""})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected ErrInvalidInput, got %v", err)
		}
	})

	t.Run("rejects unknown status", func(t *testing.T) {
		svc, _, id := setup()
		_, err := svc.Update(context.Background(), id, UpdateInput{Title: "x", Status: "weird"})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected ErrInvalidInput, got %v", err)
		}
	})

	t.Run("not found", func(t *testing.T) {
		svc, _, _ := setup()
		_, err := svc.Update(context.Background(), 9999, UpdateInput{Title: "x", Status: taskdomain.StatusNew})
		if !errors.Is(err, taskdomain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("does not call repo on validation failure", func(t *testing.T) {
		svc, repo, id := setup()
		repo.lastUpdated = nil
		_, _ = svc.Update(context.Background(), id, UpdateInput{Title: "", Status: taskdomain.StatusNew})
		if repo.lastUpdated != nil {
			t.Errorf("repo.Update should not be called when validation fails")
		}
	})
}

func TestService_Delete(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		repo := newFakeRepo()
		svc := NewService(repo)
		created, _ := svc.Create(context.Background(), CreateInput{Title: "x"})

		if err := svc.Delete(context.Background(), created.ID); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := svc.GetByID(context.Background(), created.ID); !errors.Is(err, taskdomain.ErrNotFound) {
			t.Errorf("expected ErrNotFound after delete, got %v", err)
		}
	})

	t.Run("rejects non-positive id", func(t *testing.T) {
		svc := NewService(newFakeRepo())
		for _, id := range []int64{0, -1, -1000} {
			if err := svc.Delete(context.Background(), id); !errors.Is(err, ErrInvalidInput) {
				t.Errorf("id=%d: expected ErrInvalidInput, got %v", id, err)
			}
		}
	})

	t.Run("not found", func(t *testing.T) {
		svc := NewService(newFakeRepo())
		err := svc.Delete(context.Background(), 42)
		if !errors.Is(err, taskdomain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestService_List(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		svc := NewService(newFakeRepo())
		got, err := svc.List(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("expected empty, got %d items", len(got))
		}
	})

	t.Run("returns all items", func(t *testing.T) {
		svc := NewService(newFakeRepo())
		for i := 0; i < 3; i++ {
			if _, err := svc.Create(context.Background(), CreateInput{Title: "t"}); err != nil {
				t.Fatalf("create failed: %v", err)
			}
		}
		got, err := svc.List(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 3 {
			t.Errorf("expected 3, got %d", len(got))
		}
	})

	t.Run("propagates repo error", func(t *testing.T) {
		repo := newFakeRepo()
		repo.listErr = errBoom
		svc := NewService(repo)
		_, err := svc.List(context.Background())
		if !errors.Is(err, errBoom) {
			t.Fatalf("expected errBoom, got %v", err)
		}
	})
}

// guard against accidentally widening ErrInvalidInput's message contract.
func TestErrInvalidInput_MessageStable(t *testing.T) {
	if !strings.Contains(ErrInvalidInput.Error(), "invalid") {
		t.Errorf("ErrInvalidInput message changed: %q", ErrInvalidInput.Error())
	}
}

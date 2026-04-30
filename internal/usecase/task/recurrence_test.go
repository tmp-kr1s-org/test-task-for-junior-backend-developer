package task

import (
	"context"
	"testing"
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
)

func TestService_Create_WithRepeat(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	interval := 1
	scheduled := time.Date(2026, 4, 25, 9, 0, 0, 0, time.UTC)

	tsk, err := svc.Create(context.Background(), CreateInput{
		Title:       "ward",
		Status:      taskdomain.StatusNew,
		ScheduledAt: &scheduled,
		Repeat: &RepeatInput{
			Type:         taskdomain.RuleDaily,
			StartDate:    scheduled,
			IntervalDays: &interval,
		},
	})
	if err != nil {
		t.Fatalf("create with repeat: %v", err)
	}
	if tsk.ID == 0 {
		t.Fatalf("expected an ID, got 0")
	}
	if _, has := repo.rulesByTask[tsk.ID]; !has {
		t.Fatalf("expected a rule attached to task %d", tsk.ID)
	}
}

func TestService_Create_RejectsBadRepeat(t *testing.T) {
	svc := NewService(newFakeRepo())
	scheduled := time.Date(2026, 4, 25, 9, 0, 0, 0, time.UTC)
	_, err := svc.Create(context.Background(), CreateInput{
		Title:       "x",
		ScheduledAt: &scheduled,
		Repeat:      &RepeatInput{Type: taskdomain.RuleWeekly, StartDate: scheduled}, // missing weekdays
	})
	if err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestService_ListOccurrences_ExpandsAndAppliesOverride(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	scheduled := time.Date(2026, 4, 25, 8, 0, 0, 0, time.UTC)
	created, err := svc.Create(context.Background(), CreateInput{
		Title:       "round",
		ScheduledAt: &scheduled,
		Repeat: &RepeatInput{
			Type:      taskdomain.RuleWeekly,
			StartDate: mustDate("2026-04-25"),
			Weekdays:  []int{1, 3, 5},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	doneStatus := taskdomain.StatusDone
	if err := svc.UpsertOccurrenceOverride(context.Background(), created.ID, mustDate("2026-04-29"), OverrideInput{Status: &doneStatus}); err != nil {
		t.Fatalf("upsert override: %v", err)
	}
	if err := svc.CancelOccurrence(context.Background(), created.ID, mustDate("2026-05-01")); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	occs, err := svc.ListOccurrences(context.Background(), mustDate("2026-04-25"), mustDate("2026-05-09"))
	if err != nil {
		t.Fatalf("list occurrences: %v", err)
	}
	got := []string{}
	statuses := map[string]taskdomain.Status{}
	for _, o := range occs {
		key := o.Date.Format("2006-01-02")
		got = append(got, key)
		statuses[key] = o.Status
	}
	want := []string{"2026-04-27", "2026-04-29", "2026-05-04", "2026-05-06", "2026-05-08"}
	if !equalStringSlices(got, want) {
		t.Fatalf("dates: got %v want %v", got, want)
	}
	if statuses["2026-04-29"] != taskdomain.StatusDone {
		t.Errorf("override status not applied: %v", statuses)
	}
	if statuses["2026-04-27"] != taskdomain.StatusNew {
		t.Errorf("base status mutated: %v", statuses)
	}
}

func TestService_ListOccurrences_SingleShot(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	scheduled := time.Date(2026, 4, 25, 9, 0, 0, 0, time.UTC)
	if _, err := svc.Create(context.Background(), CreateInput{
		Title:       "one off",
		ScheduledAt: &scheduled,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	occs, err := svc.ListOccurrences(context.Background(), mustDate("2026-04-20"), mustDate("2026-04-30"))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(occs) != 1 {
		t.Fatalf("want 1 occurrence, got %d", len(occs))
	}
}

func TestService_UpsertOverride_RejectsDateNotProducedByRule(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	scheduled := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	created, err := svc.Create(context.Background(), CreateInput{
		Title:       "x",
		ScheduledAt: &scheduled,
		Repeat: &RepeatInput{
			Type:      taskdomain.RuleWeekly,
			StartDate: mustDate("2026-04-25"),
			Weekdays:  []int{1}, // Mondays only
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// 2026-04-29 is a Wednesday → not produced
	err = svc.UpsertOccurrenceOverride(context.Background(), created.ID, mustDate("2026-04-29"), OverrideInput{IsCancelled: true})
	if err == nil {
		t.Fatalf("expected error for non-rule-produced date")
	}
}

func TestService_ForkSeries(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	scheduled := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	created, err := svc.Create(context.Background(), CreateInput{
		Title:       "old",
		ScheduledAt: &scheduled,
		Repeat: &RepeatInput{
			Type:      taskdomain.RuleWeekly,
			StartDate: mustDate("2026-04-25"),
			Weekdays:  []int{1, 3, 5},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	newTask, err := svc.ForkSeries(context.Background(), created.ID, mustDate("2026-05-04"), UpdateInput{
		Title:  "new",
		Status: taskdomain.StatusNew,
	}, nil)
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	if newTask.ID == created.ID {
		t.Fatalf("forked task must have a new id")
	}

	// Old series should be capped at 2026-05-03; new series should start 2026-05-04.
	if got := repo.rules[repo.rulesByTask[created.ID]].EndDate; got == nil || got.Format("2006-01-02") != "2026-05-03" {
		t.Errorf("old series end_date wrong: %v", got)
	}
	if got := repo.rules[repo.rulesByTask[newTask.ID]].StartDate.Format("2006-01-02"); got != "2026-05-04" {
		t.Errorf("new series start_date wrong: %v", got)
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

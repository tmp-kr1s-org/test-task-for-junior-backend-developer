package task

import (
	"testing"
	"time"

	taskdomain "example.com/taskservice/internal/domain/task"
)

func mustDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func datesEqual(a []time.Time, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Format("2006-01-02") != b[i] {
			return false
		}
	}
	return true
}

func TestExpandSeries_SpecificDates(t *testing.T) {
	rule := taskdomain.RepeatRule{
		Type:      taskdomain.RuleSpecificDates,
		StartDate: mustDate("2026-04-01"),
		SpecificDates: []time.Time{
			mustDate("2026-04-05"),
			mustDate("2026-04-10"),
			mustDate("2026-05-01"),
		},
	}
	got := ExpandSeries(rule, mustDate("2026-04-01"), mustDate("2026-04-30"))
	if !datesEqual(got, []string{"2026-04-05", "2026-04-10"}) {
		t.Fatalf("specific_dates: got %v", got)
	}
}

func TestExpandSeries_EvenOdd(t *testing.T) {
	from := mustDate("2026-04-01")
	to := mustDate("2026-04-07")

	even := ExpandSeries(taskdomain.RepeatRule{Type: taskdomain.RuleEvenDays, StartDate: from}, from, to)
	if !datesEqual(even, []string{"2026-04-02", "2026-04-04", "2026-04-06"}) {
		t.Fatalf("even: got %v", even)
	}

	odd := ExpandSeries(taskdomain.RepeatRule{Type: taskdomain.RuleOddDays, StartDate: from}, from, to)
	if !datesEqual(odd, []string{"2026-04-01", "2026-04-03", "2026-04-05", "2026-04-07"}) {
		t.Fatalf("odd: got %v", odd)
	}
}

func TestExpandSeries_DailyInterval(t *testing.T) {
	interval := 3
	rule := taskdomain.RepeatRule{
		Type:         taskdomain.RuleDaily,
		StartDate:    mustDate("2026-04-01"),
		IntervalDays: &interval,
	}
	got := ExpandSeries(rule, mustDate("2026-04-01"), mustDate("2026-04-15"))
	want := []string{"2026-04-01", "2026-04-04", "2026-04-07", "2026-04-10", "2026-04-13"}
	if !datesEqual(got, want) {
		t.Fatalf("daily: got %v", got)
	}
}

func TestExpandSeries_Weekly(t *testing.T) {
	// 2026-04-25 is Saturday; pick Mon=1, Wed=3, Fri=5.
	rule := taskdomain.RepeatRule{
		Type:      taskdomain.RuleWeekly,
		StartDate: mustDate("2026-04-25"),
		Weekdays:  []int{1, 3, 5},
	}
	got := ExpandSeries(rule, mustDate("2026-04-25"), mustDate("2026-05-09"))
	want := []string{"2026-04-27", "2026-04-29", "2026-05-01", "2026-05-04", "2026-05-06", "2026-05-08"}
	if !datesEqual(got, want) {
		t.Fatalf("weekly: got %v", got)
	}
}

func TestExpandSeries_Monthly(t *testing.T) {
	rule := taskdomain.RepeatRule{
		Type:      taskdomain.RuleMonthly,
		StartDate: mustDate("2026-04-01"),
		MonthDays: []int{1, 15},
	}
	got := ExpandSeries(rule, mustDate("2026-04-01"), mustDate("2026-05-31"))
	want := []string{"2026-04-01", "2026-04-15", "2026-05-01", "2026-05-15"}
	if !datesEqual(got, want) {
		t.Fatalf("monthly: got %v", got)
	}
}

func TestExpandSeries_RespectsEndDate(t *testing.T) {
	end := mustDate("2026-04-04")
	interval := 1
	rule := taskdomain.RepeatRule{
		Type:         taskdomain.RuleDaily,
		StartDate:    mustDate("2026-04-01"),
		EndDate:      &end,
		IntervalDays: &interval,
	}
	got := ExpandSeries(rule, mustDate("2026-04-01"), mustDate("2026-04-30"))
	want := []string{"2026-04-01", "2026-04-02", "2026-04-03", "2026-04-04"}
	if !datesEqual(got, want) {
		t.Fatalf("end_date: got %v", got)
	}
}

func TestExpandSeries_RespectsStartDate(t *testing.T) {
	interval := 1
	rule := taskdomain.RepeatRule{
		Type:         taskdomain.RuleDaily,
		StartDate:    mustDate("2026-04-10"),
		IntervalDays: &interval,
	}
	got := ExpandSeries(rule, mustDate("2026-04-01"), mustDate("2026-04-12"))
	want := []string{"2026-04-10", "2026-04-11", "2026-04-12"}
	if !datesEqual(got, want) {
		t.Fatalf("start_date: got %v", got)
	}
}

func TestExpandSeries_EmptyWindow(t *testing.T) {
	rule := taskdomain.RepeatRule{Type: taskdomain.RuleEvenDays, StartDate: mustDate("2026-04-01")}
	got := ExpandSeries(rule, mustDate("2026-05-01"), mustDate("2026-04-01"))
	if len(got) != 0 {
		t.Fatalf("empty window: got %v", got)
	}
}

func TestMerge_AppliesOverridesAndCancels(t *testing.T) {
	scheduled := time.Date(2026, 4, 25, 9, 30, 0, 0, time.UTC)
	series := taskdomain.Task{
		ID:          1,
		Title:       "ward round",
		Description: "morning",
		Status:      taskdomain.StatusNew,
		ScheduledAt: &scheduled,
	}

	dates := []time.Time{mustDate("2026-04-27"), mustDate("2026-04-29"), mustDate("2026-05-01")}

	doneTitle := "ward round (extended)"
	doneStatus := taskdomain.StatusDone
	overrides := map[string]taskdomain.Override{
		"2026-04-29": {Date: mustDate("2026-04-29"), Title: &doneTitle, Status: &doneStatus},
		"2026-05-01": {Date: mustDate("2026-05-01"), IsCancelled: true},
	}

	occ := Merge(series, 0, dates, overrides)
	if len(occ) != 2 {
		t.Fatalf("want 2 occurrences (one cancelled), got %d", len(occ))
	}
	if occ[0].Status != taskdomain.StatusNew || occ[0].Title != "ward round" {
		t.Errorf("base day mutated: %+v", occ[0])
	}
	if occ[1].Status != taskdomain.StatusDone || occ[1].Title != doneTitle {
		t.Errorf("override not applied: %+v", occ[1])
	}
	if occ[0].ScheduledAt == nil || occ[0].ScheduledAt.Hour() != 9 || occ[0].ScheduledAt.Day() != 27 {
		t.Errorf("scheduled_at not anchored to occurrence date: %+v", occ[0].ScheduledAt)
	}
}

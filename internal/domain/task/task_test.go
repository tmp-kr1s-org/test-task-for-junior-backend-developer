package task

import "testing"

func TestStatusValid(t *testing.T) {
	cases := []struct {
		name   string
		status Status
		want   bool
	}{
		{"new", StatusNew, true},
		{"in_progress", StatusInProgress, true},
		{"done", StatusDone, true},
		{"empty", "", false},
		{"unknown", "archived", false},
		{"case-sensitive", "NEW", false},
		{"with-space", "new ", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.status.Valid(); got != tc.want {
				t.Errorf("Status(%q).Valid() = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

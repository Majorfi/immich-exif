package model

import "testing"

func TestResultStatusString(t *testing.T) {
	testCases := []struct {
		status ResultStatus
		want   string
	}{
		{StatusSuccess, "success"},
		{StatusSkipped, "skipped"},
		{StatusFailed, "failed"},
		{ResultStatus(99), "unknown"},
	}

	for _, tc := range testCases {
		t.Run(tc.want, func(t *testing.T) {
			got := tc.status.String()
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

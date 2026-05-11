package limits

import "testing"

func TestGetRequestIPReturnsEmptyStringForNilRequest(t *testing.T) {
	if got := GetRequestIP(nil); got != "" {
		t.Fatalf("expected empty IP for nil request, got %q", got)
	}
}

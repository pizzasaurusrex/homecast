package version

import (
	"strings"
	"testing"
)

func TestStringContainsFields(t *testing.T) {
	got := String()
	for _, want := range []string{"homecast", Version, Commit, Date} {
		if !strings.Contains(got, want) {
			t.Errorf("String() = %q; missing %q", got, want)
		}
	}
}

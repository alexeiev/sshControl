package cmd

import (
	"strings"
	"testing"

	"github.com/alexeiev/sshControl/config"
)

func TestHostMatchesFilter(t *testing.T) {
	t.Parallel()

	item := hostItem{host: config.Host{Name: "web-1", Host: "10.0.0.1", Tags: []string{"web", "prod"}}}

	tests := []struct {
		filter string
		want   bool
	}{
		{"web", true},
		{"@web", true},
		{"@web @prod", true},
		{"@web @db", false},
		{"web-1 @prod", true},
		{"10.0.0 @staging", false},
		{"@pro", false},
	}

	for _, tt := range tests {
		if got := hostMatchesFilter(item, strings.Fields(tt.filter)); got != tt.want {
			t.Errorf("hostMatchesFilter(%q) = %v, want %v", tt.filter, got, tt.want)
		}
	}
}

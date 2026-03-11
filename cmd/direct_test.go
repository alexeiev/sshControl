package cmd

import (
	"os/user"
	"testing"

	"github.com/alexeiev/sshControl/config"
)

func TestParseDirectConnection(t *testing.T) {
	t.Parallel()

	effectiveUser := &config.User{Name: "ubuntu"}

	tests := []struct {
		name          string
		input         string
		effectiveUser *config.User
		wantUser      string
		wantHost      string
		wantPort      int
		wantErr       bool
	}{
		{
			name:          "host uses effective user and default port",
			input:         "srv.internal",
			effectiveUser: effectiveUser,
			wantUser:      "ubuntu",
			wantHost:      "srv.internal",
			wantPort:      22,
		},
		{
			name:          "explicit user and port override defaults",
			input:         "deploy@srv.internal:2222",
			effectiveUser: effectiveUser,
			wantUser:      "deploy",
			wantHost:      "srv.internal",
			wantPort:      2222,
		},
		{
			name:          "host and port keep effective user",
			input:         "10.0.0.10:2200",
			effectiveUser: effectiveUser,
			wantUser:      "ubuntu",
			wantHost:      "10.0.0.10",
			wantPort:      2200,
		},
		{
			name:     "falls back to system user when no effective user",
			input:    "localhost",
			wantHost: "localhost",
			wantPort: 22,
		},
		{
			name:    "rejects invalid port",
			input:   "srv.internal:70000",
			wantErr: true,
		},
		{
			name:    "rejects malformed input",
			input:   "bad@host:abc",
			wantErr: true,
		},
	}

	currentUser, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current() failed: %v", err)
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseDirectConnection(tt.input, tt.effectiveUser)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDirectConnection(%q) returned error: %v", tt.input, err)
			}

			wantUser := tt.wantUser
			if wantUser == "" {
				wantUser = currentUser.Username
			}

			if got.parsedUser != wantUser || got.hostname != tt.wantHost || got.port != tt.wantPort {
				t.Fatalf("parseDirectConnection(%q) = %+v, want user=%q host=%q port=%d", tt.input, got, wantUser, tt.wantHost, tt.wantPort)
			}
		})
	}
}

func TestValidateHostFormat(t *testing.T) {
	t.Parallel()

	if !ValidateHostFormat("ubuntu@example.com:2222") {
		t.Fatal("expected valid host format")
	}
	if ValidateHostFormat("ubuntu@example.com:abc") {
		t.Fatal("expected invalid host format")
	}
}

package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alexeiev/sshControl/config"
)

func TestResolveHostInputs(t *testing.T) {
	t.Parallel()

	cfg := &config.ConfigFile{
		Hosts: []config.Host{
			{Name: "web-1", Host: "10.0.0.1", Tags: []string{"web", "prod"}},
			{Name: "web-2", Host: "10.0.0.2", Tags: []string{"web"}},
			{Name: "db-1", Host: "10.0.0.3", Tags: []string{"db", "prod"}},
			{Name: "web-3", Host: "10.0.0.4", Tags: []string{"Web", "PROD"}},
		},
	}

	tests := []struct {
		name      string
		args      []string
		wantHosts []string
		wantTags  []string
	}{
		{
			name:      "tag única",
			args:      []string{"@web"},
			wantHosts: []string{"web-1", "web-2", "web-3"},
			wantTags:  []string{"web"},
		},
		{
			name:      "múltiplas tags usam interseção",
			args:      []string{"@web", "@prod"},
			wantHosts: []string{"web-1", "web-3"},
			wantTags:  []string{"web", "prod"},
		},
		{
			name:      "hosts diretos são mantidos junto com a interseção",
			args:      []string{"db-1", "@web", "10.9.9.9", "@prod", "db-1"},
			wantHosts: []string{"db-1", "web-1", "web-3", "10.9.9.9"},
			wantTags:  []string{"web", "prod"},
		},
		{
			name:      "tags sem interseção não retornam hosts",
			args:      []string{"@web", "@db", "db-1"},
			wantHosts: []string{"db-1"},
			wantTags:  []string{"web", "db"},
		},
		{
			name:      "tag inexistente zera a interseção",
			args:      []string{"@prod", "@missing"},
			wantHosts: nil,
			wantTags:  []string{"prod", "missing"},
		},
		{
			name:      "tags repetidas são ignoradas",
			args:      []string{"@web", "@WEB"},
			wantHosts: []string{"web-1", "web-2", "web-3"},
			wantTags:  []string{"web"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHosts, gotTags, err := ResolveHostInputs(cfg, tt.args)
			if err != nil {
				t.Fatalf("ResolveHostInputs retornou erro: %v", err)
			}
			if !reflect.DeepEqual(gotHosts, tt.wantHosts) {
				t.Fatalf("ResolveHostInputs hosts = %v, want %v", gotHosts, tt.wantHosts)
			}
			if !reflect.DeepEqual(gotTags, tt.wantTags) {
				t.Fatalf("ResolveHostInputs tags = %v, want %v", gotTags, tt.wantTags)
			}
		})
	}
}

func TestReadConfiguredPublicKeys(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	keyOne := filepath.Join(tempDir, "id_ed25519")
	keyTwo := filepath.Join(tempDir, "id_rsa")
	keyMissing := filepath.Join(tempDir, "missing")

	pubOne := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITestKey1 user@host"
	pubTwo := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQCTestKey2 user@host"

	if err := os.WriteFile(keyOne+".pub", []byte(pubOne+"\n"), 0600); err != nil {
		t.Fatalf("failed to write public key 1: %v", err)
	}
	if err := os.WriteFile(keyTwo+".pub", []byte(pubTwo+"  \n"), 0600); err != nil {
		t.Fatalf("failed to write public key 2: %v", err)
	}

	got := readConfiguredPublicKeys([]string{keyOne, keyMissing, keyTwo}, func(string, ...interface{}) {})
	want := []string{pubOne, pubTwo}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("readConfiguredPublicKeys = %v, want %v", got, want)
	}
}

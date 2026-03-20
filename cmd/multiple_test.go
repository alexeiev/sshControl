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
		},
	}

	gotHosts, gotTags, err := ResolveHostInputs(cfg, []string{"@web", "db-1", "@prod", "@missing", "db-1"})
	if err != nil {
		t.Fatalf("ResolveHostInputs retornou erro: %v", err)
	}

	wantHosts := []string{"web-1", "web-2", "db-1"}
	wantTags := []string{"web", "prod", "missing"}

	if !reflect.DeepEqual(gotHosts, wantHosts) {
		t.Fatalf("ResolveHostInputs hosts = %v, want %v", gotHosts, wantHosts)
	}
	if !reflect.DeepEqual(gotTags, wantTags) {
		t.Fatalf("ResolveHostInputs tags = %v, want %v", gotTags, wantTags)
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

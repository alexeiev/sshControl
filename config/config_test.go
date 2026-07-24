package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestApplyDefaultsAndEffectiveUser(t *testing.T) {
	t.Parallel()

	cfg := &ConfigFile{
		Config: Config{
			DefaultUser: "deploy",
			User: []User{
				{Name: "ubuntu", SSHKeys: []string{"~/.ssh/id_ed25519"}},
				{Name: "deploy", SSHKeys: []string{"~/.ssh/deploy_id"}},
			},
		},
	}

	cfg.applyDefaults()
	if cfg.Config.DirCpDefault != "~/sshControl" {
		t.Fatalf("DirCpDefault = %q, want ~/sshControl", cfg.Config.DirCpDefault)
	}

	if got := cfg.GetDefaultUser(); got == nil || got.Name != "deploy" {
		t.Fatalf("GetDefaultUser() = %+v, want deploy", got)
	}

	selected := cfg.FindUser("ubuntu")
	if got := cfg.GetEffectiveUser(selected); got == nil || got.Name != "ubuntu" {
		t.Fatalf("GetEffectiveUser(selected) = %+v, want ubuntu", got)
	}
}

func TestResolveUser(t *testing.T) {
	t.Parallel()

	cfg := &ConfigFile{
		Config: Config{
			User: []User{
				{Name: "ubuntu", SSHKeys: []string{"~/.ssh/id_rsa"}},
				{Name: "devops", SSHKeys: []string{"~/.ssh/deploy_id"}},
				{Name: "root"},
				{Name: "2fa", SSHKeys: []string{"~/.ssh/id_rsa"}},
			},
		},
	}

	// Por índice (1-based)
	if got := cfg.ResolveUser("1"); got == nil || got.Name != "ubuntu" {
		t.Fatalf("ResolveUser(\"1\") = %+v, want ubuntu", got)
	}
	if got := cfg.ResolveUser("3"); got == nil || got.Name != "root" {
		t.Fatalf("ResolveUser(\"3\") = %+v, want root", got)
	}

	// Por nome
	if got := cfg.ResolveUser("devops"); got == nil || got.Name != "devops" {
		t.Fatalf("ResolveUser(\"devops\") = %+v, want devops", got)
	}

	// Índice fora do intervalo
	if got := cfg.ResolveUser("0"); got != nil {
		t.Fatalf("ResolveUser(\"0\") = %+v, want nil", got)
	}
	if got := cfg.ResolveUser("5"); got != nil {
		t.Fatalf("ResolveUser(\"5\") = %+v, want nil", got)
	}

	// Nome alfanumérico que começa com dígito NÃO é índice: deve buscar por nome
	if got := cfg.ResolveUser("2fa"); got == nil || got.Name != "2fa" {
		t.Fatalf("ResolveUser(\"2fa\") = %+v, want usuário 2fa (nome, não índice)", got)
	}

	// Nome inexistente e string vazia
	if got := cfg.ResolveUser("missing"); got != nil {
		t.Fatalf("ResolveUser(\"missing\") = %+v, want nil", got)
	}
	if got := cfg.ResolveUser(""); got != nil {
		t.Fatalf("ResolveUser(\"\") = %+v, want nil", got)
	}

	// GetUserByIndex direto
	if got := cfg.GetUserByIndex(2); got == nil || got.Name != "devops" {
		t.Fatalf("GetUserByIndex(2) = %+v, want devops", got)
	}
	if got := cfg.GetUserByIndex(99); got != nil {
		t.Fatalf("GetUserByIndex(99) = %+v, want nil", got)
	}
}

func TestResolveJumpHostAndGetHostsForTUI(t *testing.T) {
	t.Parallel()

	cfg := &ConfigFile{
		Config: Config{
			User: []User{
				{Name: "ubuntu", SSHKeys: []string{"~/.ssh/id_ed25519", "~/.ssh/id_rsa"}},
			},
			JumpHosts: []JumpHost{
				{Name: "prod-jump", Host: "jump.example.com", User: "ubuntu", Port: 22},
			},
		},
		Hosts: []Host{
			{Name: "web-1", Host: "10.0.0.1", Tags: []string{"web"}},
			{Name: "temp-1", Host: "10.0.0.2", Tags: []string{"autocreated"}},
		},
	}

	if got := cfg.ResolveJumpHost("1"); got == nil || got.Name != "prod-jump" {
		t.Fatalf("ResolveJumpHost by index = %+v, want prod-jump", got)
	}
	if got := cfg.ResolveJumpHost("prod-jump"); got == nil || got.Host != "jump.example.com" {
		t.Fatalf("ResolveJumpHost by name = %+v, want jump.example.com", got)
	}
	if got := cfg.ResolveJumpHost("missing"); got != nil {
		t.Fatalf("ResolveJumpHost(missing) = %+v, want nil", got)
	}

	gotHosts := cfg.GetHostsForTUI()
	if len(gotHosts) != 1 || gotHosts[0].Name != "web-1" {
		t.Fatalf("GetHostsForTUI() = %+v, want only web-1", gotHosts)
	}

	gotKeys := cfg.GetJumpHostSSHKeys(cfg.ResolveJumpHost("prod-jump"))
	wantKeys := []string{ExpandHomePath("~/.ssh/id_ed25519"), ExpandHomePath("~/.ssh/id_rsa")}
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Fatalf("GetJumpHostSSHKeys() = %v, want %v", gotKeys, wantKeys)
	}
}

func TestValidateSSHKeyPairs(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	privateKey := filepath.Join(tempDir, "id_ed25519")
	if err := os.WriteFile(privateKey, []byte("private"), 0600); err != nil {
		t.Fatalf("failed to write private key: %v", err)
	}

	user := &User{
		Name: "ubuntu",
		SSHKeys: []string{
			privateKey,
			filepath.Join(tempDir, "missing"),
		},
	}

	warnings := ValidateSSHKeyPairs(user)
	if len(warnings) != 2 {
		t.Fatalf("ValidateSSHKeyPairs() returned %d warnings, want 2: %v", len(warnings), warnings)
	}
}

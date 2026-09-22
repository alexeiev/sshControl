package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateConfigJumpHostFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config string
	}{
		{
			name: "campo ausente recebe o default do template",
			config: `config:
  default_user: ubuntu
  jump_hosts:
    - name: main-jump
      host: 10.0.0.1
      user: ubuntu
      port: 22
hosts: []
`,
		},
		{
			name: "campo numérico gravado como \"\" é corrigido",
			config: `config:
  default_user: ubuntu
  jump_hosts:
    - name: main-jump
      host: 10.0.0.1
      user: ubuntu
      port: 22
      local_port_socks: ""
hosts: []
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tt.config), 0644); err != nil {
				t.Fatalf("falha ao criar config: %v", err)
			}

			if err := MigrateConfig(path); err != nil {
				t.Fatalf("MigrateConfig retornou erro: %v", err)
			}

			cfg, err := LoadConfig(path)
			if err != nil {
				data, _ := os.ReadFile(path)
				t.Fatalf("LoadConfig após migração retornou erro: %v\n%s", err, data)
			}
			if got := cfg.Config.JumpHosts[0].LocalPortSOCKS; got != 4000 {
				t.Fatalf("LocalPortSOCKS = %d, want 4000", got)
			}
		})
	}
}

func TestMigrateConfigKeepsUserValues(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.yaml")
	content := `config:
  default_user: ubuntu
  jump_hosts:
    - name: main-jump
      host: 10.0.0.1
      user: ubuntu
      port: 22
      local_port_socks: 5000
hosts:
  - name: web
    host: 10.0.0.2
    port: 22
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("falha ao criar config: %v", err)
	}

	if err := MigrateConfig(path); err != nil {
		t.Fatalf("MigrateConfig retornou erro: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig retornou erro: %v", err)
	}
	if got := cfg.Config.JumpHosts[0].LocalPortSOCKS; got != 5000 {
		t.Fatalf("LocalPortSOCKS = %d, want 5000 (valor do usuário)", got)
	}

	// Campos de texto/lista continuam recebendo valores vazios
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "tags: []") {
		t.Fatalf("esperado 'tags: []' adicionado ao host:\n%s", data)
	}
}

func TestMigrateConfigJumpHostRoutes(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.yaml")
	content := `config:
  default_user: ubuntu
  jump_hosts:
    - name: sem-rotas
      host: 10.0.0.1
      user: ubuntu
      port: 22
      local_port_socks: 4000
    - name: rotas-parciais
      host: 10.0.0.2
      user: ubuntu
      port: 22
      local_port_socks: 4001
      routes:
        gateway: 192.168.1.36
    - name: com-rotas
      host: 10.0.0.3
      user: ubuntu
      port: 22
      local_port_socks: 4002
      routes:
        gateway: 192.168.1.36
        networks: [10.0.0.0/8]
hosts: []
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("falha ao criar config: %v", err)
	}

	if err := MigrateConfig(path); err != nil {
		t.Fatalf("MigrateConfig retornou erro: %v", err)
	}
	migrated, _ := os.ReadFile(path)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig retornou erro: %v\n%s", err, migrated)
	}

	jumps := cfg.Config.JumpHosts
	if jumps[0].Routes == nil || jumps[0].Routes.Gateway != "" || len(jumps[0].Routes.Networks) != 0 || jumps[0].HasRoutes() {
		t.Fatalf("sem-rotas: routes = %+v, want gateway/networks vazios\n%s", jumps[0].Routes, migrated)
	}
	if jumps[1].Routes.Gateway != "192.168.1.36" || len(jumps[1].Routes.Networks) != 0 {
		t.Fatalf("rotas-parciais: routes = %+v, want gateway mantido e networks vazio", jumps[1].Routes)
	}
	if !jumps[2].HasRoutes() || jumps[2].Routes.Networks[0] != "10.0.0.0/8" {
		t.Fatalf("com-rotas: routes = %+v, want valores do usuário mantidos", jumps[2].Routes)
	}

	// Migração é idempotente: uma segunda execução não altera o arquivo
	if err := MigrateConfig(path); err != nil {
		t.Fatalf("segunda MigrateConfig retornou erro: %v", err)
	}
	again, _ := os.ReadFile(path)
	if string(again) != string(migrated) {
		t.Fatalf("segunda migração alterou o arquivo:\n--- antes\n%s\n--- depois\n%s", migrated, again)
	}
}

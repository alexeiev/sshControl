package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetConfigPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	tests := []struct {
		name  string
		value string
		unset bool
		want  string
	}{
		{name: "ausente", unset: true, want: filepath.Join(home, ConfigDirName, ConfigFileName)},
		{name: "vazia", want: filepath.Join(home, ConfigDirName, ConfigFileName)},
		{name: "absoluto", value: filepath.Join(t.TempDir(), "producao.yaml"), want: ""},
		{name: "relativo", value: "ambientes/producao.yaml", want: "ambientes/producao.yaml"},
		{name: "home", value: "~/ambientes/producao.yaml", want: filepath.Join(home, "ambientes", "producao.yaml")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SCCONFIG", tt.value)
			if tt.unset {
				if err := os.Unsetenv("SCCONFIG"); err != nil {
					t.Fatal(err)
				}
			}
			want := tt.want
			if want == "" {
				want = tt.value
			}
			got, err := GetConfigPath()
			if err != nil || got != want {
				t.Fatalf("GetConfigPath() = %q, %v; esperado %q", got, err, want)
			}
		})
	}
}

func TestInitializeConfigDirCreatesSelectedConfig(t *testing.T) {
	for _, custom := range []bool{false, true} {
		name := "padrao"
		if custom {
			name = "SCCONFIG"
		}
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("SCCONFIG", "")
			defaultPath := filepath.Join(home, ConfigDirName, ConfigFileName)
			want := defaultPath
			if custom {
				want = filepath.Join(t.TempDir(), "ambientes", "producao.yaml")
				t.Setenv("SCCONFIG", want)
			}
			if ConfigExists() {
				t.Fatal("configuração ainda não deveria existir")
			}
			got, err := InitializeConfigDir()
			if err != nil || got != want {
				t.Fatalf("InitializeConfigDir() = %q, %v; esperado %q", got, err, want)
			}
			data, err := os.ReadFile(want)
			if err != nil || string(data) != defaultConfigTemplate {
				t.Fatalf("template não foi criado no arquivo escolhido: %v", err)
			}
			if !ConfigExists() {
				t.Fatal("ConfigExists() não encontrou o arquivo escolhido")
			}
			if custom {
				if _, err := os.Stat(defaultPath); !os.IsNotExist(err) {
					t.Fatalf("configuração padrão não deveria ter sido criada: %v", err)
				}
			}
		})
	}
}

func TestInitializeConfigDirMigratesAndSavesSelectedConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	defaultPath := filepath.Join(home, ConfigDirName, ConfigFileName)
	if err := os.MkdirAll(filepath.Dir(defaultPath), 0755); err != nil {
		t.Fatal(err)
	}
	defaultData := []byte("config: {}\nhosts: []\n")
	if err := os.WriteFile(defaultPath, defaultData, 0600); err != nil {
		t.Fatal(err)
	}
	customPath := filepath.Join(t.TempDir(), "producao.yaml")
	t.Setenv("SCCONFIG", customPath)
	if ConfigExists() {
		t.Fatal("ConfigExists() não deve usar o arquivo padrão quando SCCONFIG está definida")
	}
	customData := []byte("config:\n  default_user: deploy\n  users:\n    - name: deploy\n  jump_hosts: []\nhosts: []\n")
	if err := os.WriteFile(customPath, customData, 0600); err != nil {
		t.Fatal(err)
	}
	path, err := InitializeConfigDir()
	if err != nil || path != customPath {
		t.Fatalf("InitializeConfigDir() = %q, %v; esperado %q", path, err, customPath)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Config.DefaultUser != "deploy" || cfg.Config.ProxyPort != 9999 {
		t.Fatalf("migração não preservou dados ou não adicionou opções: %+v", cfg.Config)
	}
	cfg.AddHost(Host{Name: "novo", Host: "10.0.0.1", Port: 22, Tags: []string{"autocreated"}})
	if err := cfg.SaveConfig(path); err != nil {
		t.Fatal(err)
	}
	cfg, err = LoadConfig(customPath)
	if err != nil || cfg.FindHost("novo") == nil {
		t.Fatalf("host não foi salvo na configuração escolhida: %v", err)
	}
	data, err := os.ReadFile(defaultPath)
	if err != nil || string(data) != string(defaultData) {
		t.Fatalf("configuração padrão foi alterada: %v", err)
	}
}

func TestInitializeConfigDirDoesNotFallbackOnInvalidPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	parent := filepath.Join(t.TempDir(), "arquivo")
	if err := os.WriteFile(parent, []byte("não é um diretório"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SCCONFIG", filepath.Join(parent, "config.yaml"))
	if _, err := InitializeConfigDir(); err == nil {
		t.Fatal("esperado erro para SCCONFIG com caminho inválido")
	}
	if _, err := os.Stat(filepath.Join(home, ConfigDirName)); !os.IsNotExist(err) {
		t.Fatalf("diretório padrão não deveria ter sido criado: %v", err)
	}
}

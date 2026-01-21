package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Constantes do sistema
const (
	// ConfigDirName é o nome do diretório de configuração no home
	ConfigDirName = ".sshControl"

	// ConfigFileName é o nome do arquivo de configuração
	ConfigFileName = "config.yaml"
)

// defaultConfigTemplate é o template do arquivo de configuração padrão
const defaultConfigTemplate = `---
config:
  default_user: ubuntu
  proxy: "192.168.0.1:3128"  # IP:PORT do proxy HTTP/HTTPS/FTP na máquina local
  proxy_port: 9999            # Porta local no host remoto para acessar o proxy
  users:
    - name: ubuntu
      ssh_keys:
        - ~/.ssh/id_rsa
        - ~/.ssh/id_ed25519
    - name: devops
      ssh_keys:
        - ~/.ssh/id_rsa
  jump_hosts:
    - name: production-jump
      host: jump.production.example.com
      user: ubuntu
      port: 22
    - name: staging-jump
      host: jump.staging.example.com
      user: ubuntu
      port: 22

hosts:
  - name: dns
    host: 192.168.1.31
    port: 22
  - name: traefik
    host: 192.168.1.32
    port: 22
...
`

// InitializeConfigDir inicializa o diretório de configuração e retorna o caminho completo
func InitializeConfigDir() (string, error) {
	// Obtém o diretório home do usuário
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("erro ao obter diretório home: %w", err)
	}

	// Caminho completo do diretório de configuração
	configDir := filepath.Join(homeDir, ConfigDirName)

	// Verifica se o diretório existe
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		// Cria o diretório
		if err := os.MkdirAll(configDir, 0755); err != nil {
			return "", fmt.Errorf("erro ao criar diretório %s: %w", configDir, err)
		}
		fmt.Printf("✓ Diretório criado: %s\n", configDir)
	}

	// Caminho completo do arquivo de configuração
	configFile := filepath.Join(configDir, ConfigFileName)

	// Verifica se o arquivo de configuração existe
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		// Cria o arquivo com o template padrão
		if err := os.WriteFile(configFile, []byte(defaultConfigTemplate), 0644); err != nil {
			return "", fmt.Errorf("erro ao criar arquivo de configuração: %w", err)
		}
		fmt.Printf("✓ Arquivo de configuração criado: %s\n", configFile)
		fmt.Println("✓ Configuração de exemplo criada. Edite o arquivo para adicionar seus hosts.")
		fmt.Println()
	}

	return configFile, nil
}

// GetConfigPath retorna o caminho completo do arquivo de configuração
func GetConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("erro ao obter diretório home: %w", err)
	}
	return filepath.Join(homeDir, ConfigDirName, ConfigFileName), nil
}

// ConfigExists verifica se o arquivo de configuração existe
func ConfigExists() bool {
	configPath, err := GetConfigPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(configPath)
	return err == nil
}

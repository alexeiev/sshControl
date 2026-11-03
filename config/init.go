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
  auto_create: false            # Se true, salva hosts não cadastrados automaticamente com tag "autocreated"
  dir_cp_default: ~/sshControl  # Diretório padrão para downloads via 'sc cp down'
  proxy: "192.168.0.1:3128"     # IP:PORT do proxy HTTP/HTTPS/FTP na máquina local
  proxy_port: 9999              # Porta local no host remoto para acessar o proxy
  users:
    - name: ubuntu
      ssh_keys:
        - ~/.ssh/ubuntu_id_rsa
        - ~/.ssh/ubuntu_id_ed25519
    - name: devops
      ssh_keys:
        - ~/.ssh/devops_id_rsa
  jump_hosts:
    - name: production-jump
      host: jump.production.example.com
      user: ubuntu
      port: 22
      local_port_socks: 4000  # Porta local do proxy SOCKS5 usada pelo 'sc tunnel' (opcional, padrão: 4000)
      routes:                 # Rotas criadas pelo 'sc tunnel' para alcançar o jump host (opcional, requer sudo)
        gateway: ""           # Equipamento que alcança a rede do jump host (ex: 192.168.1.36)
        networks: []          # Redes em CIDR (ex: [10.0.0.0/8]). Só aplica com gateway e networks preenchidos
    - name: staging-jump
      host: jump.staging.example.com
      user: ubuntu
      port: 22

hosts:
  - name: dns
    host: 192.168.1.31
    port: 22
    tags: 
      - network
      - infra
  - name: traefik
    host: 192.168.1.32
    port: 22
    tags: 
      - web
      - proxy
      - infra
...
`

// InitializeConfigDir inicializa o diretório de configuração e retorna o caminho completo
func InitializeConfigDir() (string, error) {
	configFile, err := GetConfigPath()
	if err != nil {
		return "", err
	}

	// Caminho completo do diretório de configuração
	configDir := filepath.Dir(configFile)

	// Verifica se o diretório existe
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		// Cria o diretório
		if err := os.MkdirAll(configDir, 0755); err != nil {
			return "", fmt.Errorf("erro ao criar diretório %s: %w", configDir, err)
		}
		fmt.Printf("✓ Diretório criado: %s\n", configDir)
	} else if err != nil {
		return "", fmt.Errorf("erro ao acessar diretório %s: %w", configDir, err)
	}

	// Verifica se o arquivo de configuração existe
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		// Cria o arquivo com o template padrão
		if err := os.WriteFile(configFile, []byte(defaultConfigTemplate), 0644); err != nil {
			return "", fmt.Errorf("erro ao criar arquivo de configuração: %w", err)
		}
		fmt.Printf("✓ Arquivo de configuração criado: %s\n", configFile)
		fmt.Println("✓ Configuração de exemplo criada. Edite o arquivo para adicionar seus hosts.")
		fmt.Println()
	} else if err != nil {
		return "", fmt.Errorf("erro ao acessar arquivo de configuração %s: %w", configFile, err)
	} else {
		// Config já existe - verifica se há novas opções a serem adicionadas
		if err := MigrateConfig(configFile); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  Aviso: erro ao verificar atualizações do config: %v\n", err)
		}
	}

	return configFile, nil
}

// GetConfigPath prioriza SCCONFIG; se ausente ou vazia, usa o caminho padrão.
func GetConfigPath() (string, error) {
	if configPath := os.Getenv("SCCONFIG"); configPath != "" {
		return ExpandHomePath(configPath), nil
	}
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

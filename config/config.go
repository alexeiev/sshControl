package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// User representa um usuário com suas chaves SSH
type User struct {
	Name    string   `yaml:"name"`
	SSHKeys []string `yaml:"ssh_keys"`
}

// JumpHost representa um jump host configurado
type JumpHost struct {
	Name string `yaml:"name"`
	Host string `yaml:"host"`
	User string `yaml:"user"`
	Port int    `yaml:"port"`
}

// Config representa a seção de configuração global
type Config struct {
	DefaultUser string     `yaml:"default_user"`
	User        []User     `yaml:"users"`
	JumpHosts   []JumpHost `yaml:"jump_hosts"`
	Proxy       string     `yaml:"proxy"`      // IP:PORT do proxy (ex: 10.0.230.100:8080)
	ProxyPort   int        `yaml:"proxy_port"` // Porta local no host remoto (ex: 9999)
}

// Host representa um host SSH
type Host struct {
	Name string `yaml:"name"`
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

// ConfigFile representa a estrutura completa do arquivo YAML
type ConfigFile struct {
	Config Config `yaml:"config"`
	Hosts  []Host `yaml:"hosts"`
}

// LoadConfig carrega o arquivo de configuração YAML
func LoadConfig(filename string) (*ConfigFile, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler arquivo: %w", err)
	}

	var cfg ConfigFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("erro ao parsear YAML: %w", err)
	}

	return &cfg, nil
}

// FindUser procura um usuário pelo nome
func (c *ConfigFile) FindUser(name string) *User {
	for i := range c.Config.User {
		if c.Config.User[i].Name == name {
			return &c.Config.User[i]
		}
	}
	return nil
}

// GetProxyConfig retorna a configuração de proxy validada
func (c *Config) GetProxyConfig() (address string, port int, configured bool) {
	if c.Proxy == "" {
		return "", 0, false
	}

	port = c.ProxyPort
	if port == 0 {
		port = 9999 // porta padrão
	}

	return c.Proxy, port, true
}

// GetDefaultUser retorna o primeiro usuário da configuração
func (c *ConfigFile) GetDefaultUser() *User {
	// Primeiro tenta usar o default_user configurado
	if c.Config.DefaultUser != "" {
		if user := c.FindUser(c.Config.DefaultUser); user != nil {
			return user
		}
	}

	// Fallback: retorna o primeiro usuário da lista
	if len(c.Config.User) > 0 {
		return &c.Config.User[0]
	}

	return nil
}

// GetEffectiveUser determina qual usuário usar baseado na precedência
// Prioridade: 1. selectedUser (via flag -u), 2. default_user, 3. primeiro da lista
func (c *ConfigFile) GetEffectiveUser(selectedUser *User) *User {
	if selectedUser != nil {
		return selectedUser
	}
	return c.GetDefaultUser()
}

// GetSSHKey retorna a primeira chave SSH disponível para o usuário
func (c *ConfigFile) GetSSHKey(username string) string {
	for _, user := range c.Config.User {
		if user.Name == username && len(user.SSHKeys) > 0 {
			return ExpandHomePath(user.SSHKeys[0])
		}
	}
	return ""
}

// FindHost procura um host pelo nome
func (c *ConfigFile) FindHost(name string) *Host {
	for i := range c.Hosts {
		if c.Hosts[i].Name == name {
			return &c.Hosts[i]
		}
	}
	return nil
}

// FindJumpHost procura um jump host pelo nome
func (c *ConfigFile) FindJumpHost(name string) *JumpHost {
	for i := range c.Config.JumpHosts {
		if c.Config.JumpHosts[i].Name == name {
			return &c.Config.JumpHosts[i]
		}
	}
	return nil
}

// GetJumpHostByIndex retorna um jump host pelo índice (1-based)
func (c *ConfigFile) GetJumpHostByIndex(index int) *JumpHost {
	if index < 1 || index > len(c.Config.JumpHosts) {
		return nil
	}
	return &c.Config.JumpHosts[index-1]
}

// ResolveJumpHost resolve um jump host por nome ou índice
// Aceita: "jumpname" ou "1", "2", etc.
func (c *ConfigFile) ResolveJumpHost(identifier string) *JumpHost {
	if identifier == "" {
		return nil
	}

	// Tenta parsear como número
	var index int
	_, err := fmt.Sscanf(identifier, "%d", &index)
	if err == nil {
		// É um número, busca por índice
		return c.GetJumpHostByIndex(index)
	}

	// Não é número, busca por nome
	return c.FindJumpHost(identifier)
}

// GetJumpHostSSHKey retorna a chave SSH do usuário configurado no jump host
func (c *ConfigFile) GetJumpHostSSHKey(jumpHost *JumpHost) string {
	if jumpHost == nil {
		return ""
	}

	// Busca o usuário do jump host no config
	user := c.FindUser(jumpHost.User)
	if user == nil || len(user.SSHKeys) == 0 {
		return ""
	}

	return ExpandHomePath(user.SSHKeys[0])
}

// FormatConnection formata a string de conexão SSH
func FormatConnection(user, host string, port int, sshKey string) string {
	conn := fmt.Sprintf("conexao - %s@%s:%d", user, host, port)
	if sshKey != "" {
		conn += fmt.Sprintf(" -i %s", sshKey)
	}
	return conn
}

func ExpandHomePath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return home + path[1:]
	}
	return path
}

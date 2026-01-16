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

// Config representa a seção de configuração global
type Config struct {
	DefaultUser string `yaml:"default_user"`
	User        []User `yaml:"users"`
	JumpHosts   string `yaml:"jump_hosts"`
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

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
	DefaultUser  string     `yaml:"default_user"`
	AutoCreate   bool       `yaml:"auto_create"`    // Se true, salva hosts não cadastrados automaticamente
	DirCpDefault string     `yaml:"dir_cp_default"` // Diretório padrão para downloads (ex: ~/sshControl)
	User         []User     `yaml:"users"`
	JumpHosts    []JumpHost `yaml:"jump_hosts"`
	Proxy        string     `yaml:"proxy"`      // IP:PORT do proxy (ex: 10.0.230.100:8080)
	ProxyPort    int        `yaml:"proxy_port"` // Porta local no host remoto (ex: 9999)
}

// Host representa um host SSH
type Host struct {
	Name string   `yaml:"name"`
	Host string   `yaml:"host"`
	Port int      `yaml:"port"`
	Tags []string `yaml:"tags"`
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

	// Valida pares de chaves SSH para todos os usuários
	for i := range cfg.Config.User {
		warnings := ValidateSSHKeyPairs(&cfg.Config.User[i])
		for _, warning := range warnings {
			fmt.Fprintf(os.Stderr, "⚠️  Aviso: %s\n", warning)
		}
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

// GetDownloadDir retorna o diretório padrão para downloads
// Se não configurado, retorna ~/sshControl como padrão
func (c *Config) GetDownloadDir() string {
	if c.DirCpDefault != "" {
		return ExpandHomePath(c.DirCpDefault)
	}
	// Padrão: ~/sshControl
	home, _ := os.UserHomeDir()
	return home + "/sshControl"
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

// FindHostByAddress procura um host pelo endereço (campo host)
func (c *ConfigFile) FindHostByAddress(address string) *Host {
	for i := range c.Hosts {
		if c.Hosts[i].Host == address {
			return &c.Hosts[i]
		}
	}
	return nil
}

// HasTag verifica se um host possui uma tag específica
func (h *Host) HasTag(tag string) bool {
	tagLower := strings.ToLower(tag)
	for _, t := range h.Tags {
		if strings.ToLower(t) == tagLower {
			return true
		}
	}
	return false
}

// IsAutoCreated verifica se o host foi criado automaticamente
func (h *Host) IsAutoCreated() bool {
	return h.HasTag("autocreated")
}

// AddHost adiciona um novo host à configuração (em memória)
func (c *ConfigFile) AddHost(host Host) {
	c.Hosts = append(c.Hosts, host)
}

// SaveConfig salva a configuração atual no arquivo YAML
func (c *ConfigFile) SaveConfig(filename string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("erro ao serializar configuração: %w", err)
	}

	// Adiciona o marcador YAML no início
	content := "---\n" + string(data) + "...\n"

	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		return fmt.Errorf("erro ao salvar arquivo: %w", err)
	}

	return nil
}

// FindHostsByTag retorna todos os hosts que possuem a tag especificada
func (c *ConfigFile) FindHostsByTag(tag string) []Host {
	var hosts []Host
	tagLower := strings.ToLower(tag)
	for _, host := range c.Hosts {
		for _, t := range host.Tags {
			if strings.ToLower(t) == tagLower {
				hosts = append(hosts, host)
				break
			}
		}
	}
	return hosts
}

// GetAllTags retorna todas as tags únicas cadastradas nos hosts
func (c *ConfigFile) GetAllTags() []string {
	tagSet := make(map[string]bool)
	for _, host := range c.Hosts {
		for _, tag := range host.Tags {
			tagSet[tag] = true
		}
	}

	var tags []string
	for tag := range tagSet {
		tags = append(tags, tag)
	}
	return tags
}

// GetHostsForTUI retorna hosts filtrados para exibição na TUI
// Exclui hosts com a tag "autocreated"
func (c *ConfigFile) GetHostsForTUI() []Host {
	var hosts []Host
	for _, host := range c.Hosts {
		if !host.IsAutoCreated() {
			hosts = append(hosts, host)
		}
	}
	return hosts
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

// fileExists verifica se um arquivo existe
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ValidateSSHKeyPairs valida se existem arquivos .pub correspondentes às chaves privadas
// Retorna uma lista de avisos para chaves sem par público
func ValidateSSHKeyPairs(user *User) []string {
	var warnings []string

	for _, keyPath := range user.SSHKeys {
		expandedKeyPath := ExpandHomePath(keyPath)

		// Verifica se a chave privada existe
		if !fileExists(expandedKeyPath) {
			warnings = append(warnings, fmt.Sprintf("Chave privada não encontrada para usuário '%s': %s", user.Name, keyPath))
			continue
		}

		// Verifica se o arquivo .pub correspondente existe
		pubKeyPath := expandedKeyPath + ".pub"
		if !fileExists(pubKeyPath) {
			warnings = append(warnings, fmt.Sprintf("Chave pública não encontrada para usuário '%s': %s.pub (auto-instalação desabilitada)", user.Name, keyPath))
		}
	}

	return warnings
}

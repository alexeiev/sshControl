package cmd

import (
	"fmt"
	"os"
	"os/user"
	"regexp"
	"strconv"
	"strings"

	"github.com/alexeiev/sshControl/config"
)

// Connect processa a conexão direta com um host
// Aceita vários formatos:
// 1. Nome do host do config.yaml: "dns", "traefik"
// 2. user@host:port: "ubuntu@192.168.1.50:22"
// 3. user@host: "ubuntu@192.168.1.50" (porta 22 por padrão)
// 4. host:port: "192.168.1.50:22" (usa usuário especificado ou default)
// 5. host: "192.168.1.50" (usa usuário especificado ou default e porta 22)
func Connect(cfg *config.ConfigFile, configPath string, hostArg string, selectedUser *config.User, jumpHost *config.JumpHost, command string, proxyEnabled bool, askPassword bool, envPassword bool, verbose bool) {
	var hostname string
	var port int
	var sshKeys []string
	var shouldAutoCreate bool // Flag para indicar se deve criar o host automaticamente

	// Determina o usuário efetivo (flag -u tem precedência sobre default_user)
	effectiveUser := cfg.GetEffectiveUser(selectedUser)
	if effectiveUser == nil {
		fmt.Fprintf(os.Stderr, "Erro: Nenhum usuário configurado\n")
		os.Exit(1)
	}

	// Valida chaves SSH apenas do usuário efetivo
	config.ValidateEffectiveUserSSHKeys(effectiveUser)

	username := effectiveUser.Name
	for _, key := range effectiveUser.SSHKeys {
		sshKeys = append(sshKeys, config.ExpandHomePath(key))
	}

	// Primeiro tenta encontrar no config.yaml
	if host := cfg.FindHost(hostArg); host != nil {
		hostname = host.Host
		port = host.Port
	} else {
		// Se não encontrar, tenta parsear como conexão direta
		host, err := parseDirectConnection(hostArg, effectiveUser)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erro: %v\n", err)
			fmt.Fprintf(os.Stderr, "Use o formato: user@host:port ou user@host ou host\n")
			os.Exit(1)
		}

		// Se a string incluir um usuário explícito (user@host), usa ele
		if host.parsedUser != "" && host.parsedUser != effectiveUser.Name {
			username = host.parsedUser
			// Tenta obter as chaves SSH desse usuário específico
			if userFromConfig := cfg.FindUser(username); userFromConfig != nil {
				sshKeys = nil // Limpa chaves anteriores
				for _, key := range userFromConfig.SSHKeys {
					sshKeys = append(sshKeys, config.ExpandHomePath(key))
				}
			} else {
				// Usuário não está no config, não usa chave SSH
				sshKeys = nil
			}
		}

		hostname = host.hostname
		port = host.port

		// Verifica se auto_create está habilitado e se o host não existe pelo endereço
		if cfg.Config.AutoCreate && cfg.FindHostByAddress(hostname) == nil {
			shouldAutoCreate = true
		}
	}

	// Busca as chaves SSH do jump host se estiver usando jump host
	var jumpHostSSHKeys []string
	if jumpHost != nil {
		jumpHostSSHKeys = cfg.GetJumpHostSSHKeys(jumpHost)
	}

	// Obtém configuração de proxy
	proxyAddress, proxyPort, proxyConfigured := cfg.Config.GetProxyConfig()
	proxyActive := proxyEnabled && proxyConfigured

	if !proxyActive && proxyEnabled {
		fmt.Fprintf(os.Stderr, "⚠️  Aviso: Proxy solicitado mas não configurado no config.yaml\n")
	}

	// Resolve a senha: -P lê da variável SCPW, -a solicita interativamente
	password, err := ResolvePassword(askPassword, envPassword, fmt.Sprintf("Password for %s@%s: ", username, hostname))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erro: %v\n", err)
		os.Exit(1)
	}

	// Cria e executa a conexão SSH
	sshConn := NewSSHConnection(
		username,
		hostname,
		port,
		sshKeys,
		password, // Senha (vazia se -a não for especificado, ou fornecida pelo usuário)
		jumpHost,
		jumpHostSSHKeys,
		command,
		proxyActive,
		proxyAddress,
		proxyPort,
		verbose,
	)

	// Decide se executa comando remoto ou inicia sessão interativa
	if command != "" {
		err = sshConn.ExecuteCommand()
	} else {
		err = sshConn.Connect()
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ Erro na conexão SSH: %v\n", err)
		os.Exit(1)
	}

	// Auto-criação do host após conexão bem-sucedida
	if shouldAutoCreate {
		autoCreateHost(cfg, configPath, hostArg, hostname, port)
	}
}

// autoCreateHost adiciona um host não cadastrado ao arquivo de configuração
func autoCreateHost(cfg *config.ConfigFile, configPath string, name string, hostname string, port int) {
	newHost := config.Host{
		Name: name,
		Host: hostname,
		Port: port,
		Tags: []string{"autocreated"},
	}

	cfg.AddHost(newHost)

	if err := cfg.SaveConfig(configPath); err != nil {
		fmt.Fprintf(os.Stderr, "\n⚠️  Aviso: Não foi possível salvar o host no config.yaml: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("✅ Host adicionado automaticamente ao config.yaml:\n")
	fmt.Printf("   name: %s\n", name)
	fmt.Printf("   host: %s\n", hostname)
	fmt.Printf("   port: %d\n", port)
	fmt.Printf("   tags: [autocreated]\n")
	fmt.Println()
	fmt.Println("📝 Finalize a configuração do host editando o arquivo:")
	fmt.Printf("   %s\n", configPath)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

// parsedHost representa um host parseado de uma string de conexão
type parsedHost struct {
	parsedUser string
	hostname   string
	port       int
}

// parseDirectConnection analisa uma string de conexão direta
func parseDirectConnection(input string, effectiveUser *config.User) (*parsedHost, error) {
	// Regex para parsear: [user@]host[:port]
	re := regexp.MustCompile(`^(?:([^@]+)@)?([^:]+)(?::(\d+))?$`)
	matches := re.FindStringSubmatch(input)

	if matches == nil {
		return nil, fmt.Errorf("formato inválido: '%s'", input)
	}

	parsedUser := matches[1]
	hostname := matches[2]
	portStr := matches[3]

	// Prioridade do usuário:
	// 1. Usuário especificado na string (user@host)
	// 2. Usuário efetivo (da flag -u ou default_user)
	// 3. Usuário do sistema como fallback
	if parsedUser == "" {
		if effectiveUser != nil {
			parsedUser = effectiveUser.Name
		} else {
			currentUser, err := user.Current()
			if err != nil {
				parsedUser = "root" // fallback
			} else {
				parsedUser = currentUser.Username
			}
		}
	}

	// Se não especificou porta, usa 22
	port := 22
	if portStr != "" {
		var err error
		port, err = strconv.Atoi(portStr)
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("porta inválida: '%s'", portStr)
		}
	}

	// Valida o hostname
	if hostname == "" {
		return nil, fmt.Errorf("hostname não pode ser vazio")
	}

	return &parsedHost{
		parsedUser: parsedUser,
		hostname:   hostname,
		port:       port,
	}, nil
}

// ValidateHostFormat valida se o formato da string é válido
func ValidateHostFormat(input string) bool {
	_, err := parseDirectConnection(input, nil)
	return err == nil
}

// ParseConnectionString é uma função auxiliar pública para testes
func ParseConnectionString(input string) (user, host string, port int, err error) {
	h, e := parseDirectConnection(input, nil)
	if e != nil {
		return "", "", 0, e
	}
	return h.parsedUser, h.hostname, h.port, nil
}

// ListServers exibe todos os servidores e jump hosts cadastrados no config
// Se tagFilter não estiver vazio, filtra os servidores pela tag especificada
func ListServers(cfg *config.ConfigFile, tagFilter string) {
	fmt.Println()

	// Exibe Jump Hosts se houver algum (sempre mostra, independente do filtro)
	if len(cfg.Config.JumpHosts) > 0 {
		fmt.Println("🔗 Jump Hosts cadastrados:")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Printf("%-5s %-20s %-15s %s\n", "Idx", "Nome", "Usuário", "Host:Porta")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		for i, jh := range cfg.Config.JumpHosts {
			hostPort := fmt.Sprintf("%s:%d", jh.Host, jh.Port)
			fmt.Printf("%-5d %-20s %-15s %s\n", i+1, jh.Name, jh.User, hostPort)
		}

		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Printf("Total: %d jump host(s)\n", len(cfg.Config.JumpHosts))
		fmt.Println()
	} else {
		fmt.Println("ℹ️  Nenhum jump host cadastrado no config.yaml")
		fmt.Println()
	}

	// Filtra servidores por tag se especificado
	var hostsToShow []config.Host
	if tagFilter != "" {
		hostsToShow = cfg.FindHostsByTag(tagFilter)
	} else {
		hostsToShow = cfg.Hosts
	}

	// Exibe Servidores
	if len(hostsToShow) == 0 {
		if tagFilter != "" {
			fmt.Printf("ℹ️  Nenhum servidor encontrado com a tag '%s'\n", tagFilter)
		} else {
			fmt.Println("ℹ️  Nenhum servidor cadastrado no config.yaml")
		}
		fmt.Println()
		return
	}

	if tagFilter != "" {
		fmt.Printf("📋 Servidores com tag '%s':\n", tagFilter)
	} else {
		fmt.Println("📋 Servidores cadastrados:")
	}
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("%-20s %-25s %s\n", "Nome", "Host:Porta", "Tags")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	for _, host := range hostsToShow {
		hostPort := fmt.Sprintf("%s:%d", host.Host, host.Port)
		tags := "-"
		if len(host.Tags) > 0 {
			tags = strings.Join(host.Tags, ", ")
		}
		fmt.Printf("%-20s %-25s %s\n", host.Name, hostPort, tags)
	}

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Total: %d servidor(es)\n", len(hostsToShow))
	fmt.Println()
}

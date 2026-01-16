package cmd

import (
	"fmt"
	"os"
	"os/user"
	"regexp"
	"strconv"

	"github.com/ceiev/sshControl/config"
)

// Connect processa a conexão direta com um host
// Aceita vários formatos:
// 1. Nome do host do config.yaml: "dns", "traefik"
// 2. user@host:port: "ubuntu@192.168.1.50:22"
// 3. user@host: "ubuntu@192.168.1.50" (porta 22 por padrão)
// 4. host:port: "192.168.1.50:22" (usa usuário especificado ou default)
// 5. host: "192.168.1.50" (usa usuário especificado ou default e porta 22)
func Connect(cfg *config.ConfigFile, hostArg string, selectedUser *config.User, useJumpHost bool) {
	var hostname string
	var port int
	var sshKey string

	// Determina o usuário efetivo (flag -u tem precedência sobre default_user)
	effectiveUser := cfg.GetEffectiveUser(selectedUser)
	if effectiveUser == nil {
		fmt.Fprintf(os.Stderr, "Erro: Nenhum usuário configurado\n")
		os.Exit(1)
	}

	username := effectiveUser.Name
	if len(effectiveUser.SSHKeys) > 0 {
		sshKey = config.ExpandHomePath(effectiveUser.SSHKeys[0])
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
			// Tenta obter a chave SSH desse usuário específico
			if userFromConfig := cfg.FindUser(username); userFromConfig != nil {
				if len(userFromConfig.SSHKeys) > 0 {
					sshKey = config.ExpandHomePath(userFromConfig.SSHKeys[0])
				}
			} else {
				// Usuário não está no config, não usa chave SSH
				sshKey = ""
			}
		}

		hostname = host.hostname
		port = host.port
	}

	// Prepara o Jump Host
	jumpHost := ""
	if useJumpHost {
		jumpHost = cfg.Config.JumpHosts
	}

	// Cria e executa a conexão SSH
	sshConn := NewSSHConnection(
		username,
		hostname,
		port,
		sshKey,
		useJumpHost,
		jumpHost,
	)

	if err := sshConn.Connect(); err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ Erro na conexão SSH: %v\n", err)
		os.Exit(1)
	}
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

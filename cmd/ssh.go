package cmd

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

// SSHConnection representa os parâmetros de uma conexão SSH
type SSHConnection struct {
	User        string
	Host        string
	Port        int
	SSHKey      string
	JumpHost    string
	UseJumpHost bool
	Command     string
}

// Connect estabelece uma conexão SSH interativa
func (s *SSHConnection) Connect() error {
	// Exibe a string de conexão antes de conectar
	fmt.Println()
	fmt.Println("🔗 Conectando...")
	fmt.Printf("   %s\n", s.formatConnectionString())
	fmt.Println()

	// Cria a configuração SSH
	config, err := s.createSSHConfig()
	if err != nil {
		return fmt.Errorf("erro ao criar configuração SSH: %w", err)
	}

	// Conecta ao host (via Jump Host se necessário)
	client, err := s.dial(config)
	if err != nil {
		return fmt.Errorf("erro ao conectar: %w", err)
	}
	defer client.Close()

	// Cria uma sessão SSH
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("erro ao criar sessão: %w", err)
	}
	defer session.Close()

	// Inicia a sessão interativa
	if err := s.startInteractiveSession(session); err != nil {
		return fmt.Errorf("erro na sessão interativa: %w", err)
	}

	return nil
}

// ExecuteCommand executa um comando remoto e exibe a saída
func (s *SSHConnection) ExecuteCommand() error {
	// Exibe a string de conexão e o comando antes de conectar
	fmt.Println()
	fmt.Println("🔗 Conectando...")
	fmt.Printf("   %s\n", s.formatConnectionString())
	fmt.Printf("   Comando: %s\n", s.Command)
	fmt.Println()

	// Cria a configuração SSH
	config, err := s.createSSHConfig()
	if err != nil {
		return fmt.Errorf("erro ao criar configuração SSH: %w", err)
	}

	// Conecta ao host (via Jump Host se necessário)
	client, err := s.dial(config)
	if err != nil {
		return fmt.Errorf("erro ao conectar: %w", err)
	}
	defer client.Close()

	// Cria uma sessão SSH
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("erro ao criar sessão: %w", err)
	}
	defer session.Close()

	// Conecta stdout e stderr à saída do terminal
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	// Executa o comando
	if err := session.Run(s.Command); err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			return fmt.Errorf("comando encerrado com código: %d", exitErr.ExitStatus())
		}
		return fmt.Errorf("erro ao executar comando: %w", err)
	}

	return nil
}

// createSSHConfig cria a configuração do cliente SSH
func (s *SSHConnection) createSSHConfig() (*ssh.ClientConfig, error) {
	return s.createSSHConfigWithContext(fmt.Sprintf("%s@%s:%d", s.User, s.Host, s.Port))
}

// createSSHConfigWithContext cria a configuração do cliente SSH com contexto para prompts
func (s *SSHConnection) createSSHConfigWithContext(context string) (*ssh.ClientConfig, error) {
	authMethods := []ssh.AuthMethod{}

	// Adiciona autenticação por chave SSH se especificada
	if s.SSHKey != "" {
		key, err := os.ReadFile(s.SSHKey)
		if err != nil {
			return nil, fmt.Errorf("erro ao ler chave SSH %s: %w", s.SSHKey, err)
		}

		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("erro ao parsear chave SSH: %w", err)
		}

		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}

	// Adiciona autenticação via SSH Agent se disponível
	if agentAuth := s.getSSHAgentAuth(); agentAuth != nil {
		authMethods = append(authMethods, agentAuth)
	}

	// Sempre adiciona senha interativa como fallback final
	// Será solicitada apenas se todos os métodos anteriores falharem
	authMethods = append(authMethods, ssh.PasswordCallback(func() (string, error) {
		fmt.Printf("Password for %s: ", context)
		password, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", err
		}
		return string(password), nil
	}))

	config := &ssh.ClientConfig{
		User:            s.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // Para produção, use ssh.FixedHostKey
	}

	return config, nil
}

// dial conecta ao host (via Jump Host se necessário)
func (s *SSHConnection) dial(config *ssh.ClientConfig) (*ssh.Client, error) {
	address := fmt.Sprintf("%s:%d", s.Host, s.Port)

	// Conexão direta se não usar Jump Host
	if !s.UseJumpHost || s.JumpHost == "" {
		return ssh.Dial("tcp", address, config)
	}

	// Cria configuração separada para Jump Host com contexto claro
	jumpConfig, err := s.createSSHConfigWithContext(fmt.Sprintf("%s@%s (Jump Host)", s.User, s.JumpHost))
	if err != nil {
		return nil, fmt.Errorf("erro ao criar configuração para Jump Host: %w", err)
	}

	// Conecta ao Jump Host (assume porta 22)
	jumpClient, err := ssh.Dial("tcp", net.JoinHostPort(s.JumpHost, "22"), jumpConfig)
	if err != nil {
		return nil, fmt.Errorf("erro ao conectar ao Jump Host %s: %w", s.JumpHost, err)
	}

	// Conecta ao host final através do Jump Host
	conn, err := jumpClient.Dial("tcp", address)
	if err != nil {
		jumpClient.Close()
		return nil, fmt.Errorf("erro ao conectar ao host através do Jump Host: %w", err)
	}

	// Cria o cliente SSH sobre a conexão do Jump Host (com config do target)
	ncc, chans, reqs, err := ssh.NewClientConn(conn, address, config)
	if err != nil {
		conn.Close()
		jumpClient.Close()
		return nil, fmt.Errorf("erro ao criar conexão SSH: %w", err)
	}

	return ssh.NewClient(ncc, chans, reqs), nil
}

// startInteractiveSession inicia uma sessão SSH interativa
func (s *SSHConnection) startInteractiveSession(session *ssh.Session) error {
	// Salva o estado original do terminal
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("erro ao configurar terminal: %w", err)
	}
	defer term.Restore(fd, oldState)

	// Obtém o tamanho do terminal
	width, height, err := term.GetSize(fd)
	if err != nil {
		width = 80
		height = 24
	}

	// Configura os modos do terminal
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	// Solicita um pseudo-terminal
	if err := session.RequestPty("xterm-256color", height, width, modes); err != nil {
		return fmt.Errorf("erro ao solicitar PTY: %w", err)
	}

	// Conecta stdin, stdout e stderr
	session.Stdin = os.Stdin
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	// Monitora mudanças no tamanho do terminal
	go s.monitorTerminalResize(session, fd)

	// Inicia o shell
	if err := session.Shell(); err != nil {
		return fmt.Errorf("erro ao iniciar shell: %w", err)
	}

	// Aguarda o término da sessão
	if err := session.Wait(); err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			return fmt.Errorf("sessão encerrada com código: %d", exitErr.ExitStatus())
		}
		return fmt.Errorf("erro durante sessão: %w", err)
	}

	return nil
}

// monitorTerminalResize monitora mudanças no tamanho do terminal
func (s *SSHConnection) monitorTerminalResize(session *ssh.Session, fd int) {
	sigwinch := make(chan os.Signal, 1)
	signal.Notify(sigwinch, syscall.SIGWINCH)

	for range sigwinch {
		width, height, err := term.GetSize(fd)
		if err != nil {
			continue
		}
		session.WindowChange(height, width)
	}
}

// getSSHAgentAuth tenta obter autenticação via SSH Agent
func (s *SSHConnection) getSSHAgentAuth() ssh.AuthMethod {
	socket := os.Getenv("SSH_AUTH_SOCK")
	if socket == "" {
		return nil
	}

	conn, err := net.Dial("unix", socket)
	if err != nil {
		return nil
	}

	agentClient := NewSSHAgentClient(conn)
	return ssh.PublicKeysCallback(agentClient.Signers)
}

// SSHAgentClient é um wrapper simples para o SSH Agent
type SSHAgentClient struct {
	conn net.Conn
}

func NewSSHAgentClient(conn net.Conn) *SSHAgentClient {
	return &SSHAgentClient{conn: conn}
}

func (a *SSHAgentClient) Signers() ([]ssh.Signer, error) {
	// Implementação básica - na prática, use golang.org/x/crypto/ssh/agent
	return nil, nil
}

// formatConnectionString formata a string de conexão para exibição
func (s *SSHConnection) formatConnectionString() string {
	conn := fmt.Sprintf("%s@%s", s.User, s.Host)

	if s.Port != 22 {
		conn += fmt.Sprintf(":%d", s.Port)
	}

	if s.SSHKey != "" {
		conn += fmt.Sprintf(" (key: %s)", s.SSHKey)
	}

	if s.UseJumpHost && s.JumpHost != "" {
		conn += fmt.Sprintf(" via %s", s.JumpHost)
	}

	return conn
}

// NewSSHConnection cria uma nova conexão SSH
func NewSSHConnection(user, host string, port int, sshKey string, useJumpHost bool, jumpHost string, command string) *SSHConnection {
	return &SSHConnection{
		User:        user,
		Host:        host,
		Port:        port,
		SSHKey:      sshKey,
		JumpHost:    jumpHost,
		UseJumpHost: useJumpHost,
		Command:     command,
	}
}

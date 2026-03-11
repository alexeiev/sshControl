package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"

	"github.com/alexeiev/sshControl/config"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

// SSHConnection representa os parâmetros de uma conexão SSH
type SSHConnection struct {
	User                       string
	Host                       string
	Port                       int
	SSHKeys                    []string // Múltiplas chaves SSH para tentar autenticação
	Password                   string   // Senha pré-fornecida (opcional)
	JumpHost                   *config.JumpHost
	JumpHostSSHKeys            []string // Múltiplas chaves SSH para o jump host
	Command                    string
	ProxyEnabled               bool
	ProxyAddress               string
	ProxyPort                  int
	InteractivePasswordAllowed bool // Se false, não pede senha interativamente (para modo múltiplos hosts)
	Verbose                    bool // Modo debug: exibe informações detalhadas da conexão
}

// debugLog imprime mensagens de debug quando o modo verbose está ativo
func (s *SSHConnection) debugLog(format string, args ...interface{}) {
	if s.Verbose {
		fmt.Fprintf(os.Stderr, "[DEBUG] "+format+"\n", args...)
	}
}

// Connect estabelece uma conexão SSH interativa
func (s *SSHConnection) Connect() error {
	// Exibe a string de conexão antes de conectar
	fmt.Println()
	fmt.Println("🔗 Conectando...")
	fmt.Printf("   %s\n", s.formatConnectionString())
	fmt.Println()

	s.debugLog("Iniciando conexão interativa")
	s.debugLog("Usuário: %s", s.User)
	s.debugLog("Host: %s:%d", s.Host, s.Port)
	if s.JumpHost != nil {
		s.debugLog("Jump Host: %s (%s@%s:%d)", s.JumpHost.Name, s.JumpHost.User, s.JumpHost.Host, s.JumpHost.Port)
	}
	if s.ProxyEnabled {
		s.debugLog("Proxy habilitado: %s via porta remota %d", s.ProxyAddress, s.ProxyPort)
	}

	// Cria a configuração SSH
	s.debugLog("Criando configuração SSH...")
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
	s.debugLog("Conexão SSH estabelecida com sucesso")

	// Tenta instalar a chave pública se necessário
	if err := s.installPublicKeyIfNeeded(client); err != nil {
		// Log warning mas não bloqueia
		fmt.Fprintf(os.Stderr, "⚠️  Aviso: Não foi possível instalar chave pública: %v\n", err)
	}

	// Configura remote forwarding se proxy estiver habilitado
	if s.ProxyEnabled {
		s.debugLog("Configurando proxy reverso (remote forwarding)...")
		if err := s.setupRemoteForwarding(client); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  Aviso: Não foi possível configurar proxy forwarding: %v\n", err)
		} else {
			fmt.Printf("\n✅ Proxy tunnel ativo!\n")
			fmt.Printf("   Execute no host remoto:\n")
			fmt.Printf("   export {https,http,ftp}_proxy=http://127.0.0.1:%d\n\n", s.ProxyPort)
		}
	}

	// Cria uma sessão SSH
	s.debugLog("Criando sessão SSH...")
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("erro ao criar sessão: %w", err)
	}
	defer session.Close()

	// Inicia a sessão interativa
	s.debugLog("Iniciando sessão interativa...")
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

	s.debugLog("Iniciando execução de comando remoto")
	s.debugLog("Usuário: %s", s.User)
	s.debugLog("Host: %s:%d", s.Host, s.Port)
	s.debugLog("Comando: %s", s.Command)

	// Cria a configuração SSH
	s.debugLog("Criando configuração SSH...")
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
	s.debugLog("Conexão SSH estabelecida com sucesso")

	// Tenta instalar a chave pública se necessário
	if err := s.installPublicKeyIfNeeded(client); err != nil {
		// Log warning mas não bloqueia
		fmt.Fprintf(os.Stderr, "⚠️  Aviso: Não foi possível instalar chave pública: %v\n", err)
	}

	// Cria uma sessão SSH
	s.debugLog("Criando sessão SSH...")
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("erro ao criar sessão: %w", err)
	}
	defer session.Close()

	// Conecta stdout e stderr à saída do terminal
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	// Executa o comando
	s.debugLog("Executando comando...")
	if err := session.Run(s.Command); err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			s.debugLog("Comando encerrado com exit code: %d", exitErr.ExitStatus())
			return fmt.Errorf("comando encerrado com código: %d", exitErr.ExitStatus())
		}
		return fmt.Errorf("erro ao executar comando: %w", err)
	}
	s.debugLog("Comando executado com sucesso (exit code: 0)")

	return nil
}

// createSSHConfig cria a configuração do cliente SSH
func (s *SSHConnection) createSSHConfig() (*ssh.ClientConfig, error) {
	return s.createSSHConfigWithContext(fmt.Sprintf("%s@%s:%d", s.User, s.Host, s.Port))
}

// createAuthMethods cria os métodos de autenticação para SSH
func (s *SSHConnection) createAuthMethods(sshKeyPaths []string, context string) []ssh.AuthMethod {
	authMethods := []ssh.AuthMethod{}
	var authNames []string

	// Adiciona autenticação por chaves SSH (tenta todas as chaves configuradas)
	var signers []ssh.Signer
	for _, sshKeyPath := range sshKeyPaths {
		if sshKeyPath == "" {
			continue
		}
		key, err := os.ReadFile(sshKeyPath)
		if err != nil {
			s.debugLog("Chave SSH: %s ... falha ao ler arquivo: %v", sshKeyPath, err)
			continue
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			s.debugLog("Chave SSH: %s ... falha ao parsear: %v", sshKeyPath, err)
			continue
		}
		s.debugLog("Chave SSH: %s ... OK", sshKeyPath)
		signers = append(signers, signer)
	}
	if len(signers) > 0 {
		authMethods = append(authMethods, ssh.PublicKeys(signers...))
		authNames = append(authNames, fmt.Sprintf("publickey (%d chave(s))", len(signers)))
	}

	// Adiciona autenticação via SSH Agent se disponível
	if agentAuth := s.getSSHAgentAuth(); agentAuth != nil {
		authMethods = append(authMethods, agentAuth)
		s.debugLog("SSH Agent: disponível")
		authNames = append(authNames, "agent")
	} else {
		s.debugLog("SSH Agent: não disponível")
	}

	// Adiciona autenticação por senha
	if s.Password != "" {
		// Se a senha foi pré-fornecida, usa ela diretamente
		authMethods = append(authMethods, ssh.Password(s.Password))
		s.debugLog("Senha: pré-fornecida")
		authNames = append(authNames, "password (pré-fornecida)")
	} else if s.InteractivePasswordAllowed {
		// Só pede senha interativamente se permitido (modo single host)
		// Em modo múltiplos hosts, isso é desabilitado para evitar múltiplos prompts
		authMethods = append(authMethods, ssh.PasswordCallback(func() (string, error) {
			fmt.Printf("Password for %s: ", context)
			password, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Println()
			if err != nil {
				return "", err
			}
			return string(password), nil
		}))
		s.debugLog("Senha: interativa (será solicitada se necessário)")
		authNames = append(authNames, "password (interativa)")
	}

	s.debugLog("Métodos de autenticação: [%s]", strings.Join(authNames, ", "))

	return authMethods
}

// createSSHConfigWithContext cria a configuração do cliente SSH com contexto para prompts
func (s *SSHConnection) createSSHConfigWithContext(context string) (*ssh.ClientConfig, error) {
	authMethods := s.createAuthMethods(s.SSHKeys, context)

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
	if s.JumpHost == nil {
		s.debugLog("Conectando diretamente a %s...", address)
		client, err := ssh.Dial("tcp", address, config)
		if err != nil {
			s.debugLog("Falha na conexão direta: %v", err)
			return nil, err
		}
		s.debugLog("Conexão direta estabelecida")
		return client, nil
	}

	// Cria métodos de autenticação específicos para o Jump Host
	s.debugLog("Preparando autenticação do Jump Host: %s (%s@%s:%d)", s.JumpHost.Name, s.JumpHost.User, s.JumpHost.Host, s.JumpHost.Port)
	jumpAuthMethods := s.createAuthMethods(s.JumpHostSSHKeys, fmt.Sprintf("%s@%s (Jump Host)", s.JumpHost.User, s.JumpHost.Host))

	// Cria configuração separada para Jump Host
	jumpConfig := &ssh.ClientConfig{
		User:            s.JumpHost.User,
		Auth:            jumpAuthMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	// Conecta ao Jump Host
	jumpAddress := fmt.Sprintf("%s:%d", s.JumpHost.Host, s.JumpHost.Port)
	s.debugLog("Conectando ao Jump Host %s...", jumpAddress)
	jumpClient, err := ssh.Dial("tcp", jumpAddress, jumpConfig)
	if err != nil {
		s.debugLog("Falha na conexão ao Jump Host: %v", err)
		return nil, fmt.Errorf("erro ao conectar ao Jump Host %s: %w", s.JumpHost.Name, err)
	}
	s.debugLog("Jump Host conectado, criando tunnel para %s...", address)

	// Conecta ao host final através do Jump Host
	conn, err := jumpClient.Dial("tcp", address)
	if err != nil {
		jumpClient.Close()
		s.debugLog("Falha ao criar tunnel: %v", err)
		return nil, fmt.Errorf("erro ao conectar ao host através do Jump Host: %w", err)
	}

	// Cria o cliente SSH sobre a conexão do Jump Host (com config do target)
	ncc, chans, reqs, err := ssh.NewClientConn(conn, address, config)
	if err != nil {
		conn.Close()
		jumpClient.Close()
		s.debugLog("Falha ao criar conexão SSH sobre tunnel: %v", err)
		return nil, fmt.Errorf("erro ao criar conexão SSH: %w", err)
	}

	s.debugLog("Tunnel estabelecido com sucesso")
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
	s.debugLog("Solicitando PTY (xterm-256color, %dx%d)", width, height)
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
		s.debugLog("SSH_AUTH_SOCK não definido")
		return nil
	}

	s.debugLog("SSH_AUTH_SOCK: %s", socket)
	conn, err := net.Dial("unix", socket)
	if err != nil {
		s.debugLog("Falha ao conectar ao SSH Agent: %v", err)
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

// setupRemoteForwarding configura o tunnel SSH reverso para o proxy
func (s *SSHConnection) setupRemoteForwarding(client *ssh.Client) error {
	// Remote forwarding: host remoto porta ProxyPort -> proxy local ProxyAddress
	remoteAddr := fmt.Sprintf("127.0.0.1:%d", s.ProxyPort)

	s.debugLog("Remote forwarding: %s -> %s", remoteAddr, s.ProxyAddress)
	listener, err := client.Listen("tcp", remoteAddr)
	if err != nil {
		s.debugLog("Falha ao criar listener remoto: %v", err)
		return fmt.Errorf("erro ao criar listener remoto: %w", err)
	}
	s.debugLog("Listener remoto criado em %s", remoteAddr)

	// Goroutine para aceitar conexões e fazer forwarding
	go func() {
		defer listener.Close()
		for {
			remoteConn, err := listener.Accept()
			if err != nil {
				return
			}

			// Para cada conexão remota, conecta ao proxy local
			go s.handleProxyForwarding(remoteConn)
		}
	}()

	return nil
}

// handleProxyForwarding encaminha o tráfego entre a conexão remota e o proxy local
func (s *SSHConnection) handleProxyForwarding(remoteConn net.Conn) {
	defer remoteConn.Close()

	// Conecta ao proxy local
	proxyConn, err := net.Dial("tcp", s.ProxyAddress)
	if err != nil {
		return
	}
	defer proxyConn.Close()

	// Copia dados bidirecional
	go io.Copy(proxyConn, remoteConn)
	io.Copy(remoteConn, proxyConn)
}

// formatConnectionString formata a string de conexão para exibição
func (s *SSHConnection) formatConnectionString() string {
	conn := fmt.Sprintf("%s@%s", s.User, s.Host)

	if s.Port != 22 {
		conn += fmt.Sprintf(":%d", s.Port)
	}

	if len(s.SSHKeys) > 0 {
		if len(s.SSHKeys) == 1 {
			conn += fmt.Sprintf(" (key: %s)", s.SSHKeys[0])
		} else {
			conn += fmt.Sprintf(" (keys: %d configured)", len(s.SSHKeys))
		}
	}

	if s.JumpHost != nil {
		conn += fmt.Sprintf(" via %s (%s@%s:%d)", s.JumpHost.Name, s.JumpHost.User, s.JumpHost.Host, s.JumpHost.Port)
	}

	if s.ProxyEnabled {
		conn += fmt.Sprintf(" [Proxy: %s via :%d]", s.ProxyAddress, s.ProxyPort)
	}

	return conn
}

// readPublicKey lê o conteúdo do arquivo de chave pública correspondente à chave privada
func readPublicKey(privateKeyPath string) (string, error) {
	if privateKeyPath == "" {
		return "", fmt.Errorf("caminho da chave privada está vazio")
	}

	pubKeyPath := privateKeyPath + ".pub"
	content, err := os.ReadFile(pubKeyPath)
	if err != nil {
		return "", fmt.Errorf("erro ao ler arquivo de chave pública %s: %w", pubKeyPath, err)
	}

	// Remove espaços em branco e quebras de linha no final
	pubKey := string(content)
	pubKey = string(bytes.TrimSpace([]byte(pubKey)))

	if pubKey == "" {
		return "", fmt.Errorf("arquivo de chave pública está vazio: %s", pubKeyPath)
	}

	return pubKey, nil
}

func readConfiguredPublicKeys(sshKeys []string, debugLog func(string, ...interface{})) []string {
	var pubKeys []string

	for _, sshKey := range sshKeys {
		pubKeyPath := sshKey + ".pub"
		if _, err := os.Stat(pubKeyPath); os.IsNotExist(err) {
			debugLog("Chave pública não encontrada: %s", pubKeyPath)
			continue
		}

		pubKey, err := readPublicKey(sshKey)
		if err != nil {
			debugLog("Falha ao ler chave pública %s: %v", pubKeyPath, err)
			continue
		}

		debugLog("Chave pública carregada: %s", pubKeyPath)
		pubKeys = append(pubKeys, pubKey)
	}

	return pubKeys
}

// installPublicKeyIfNeeded instala a chave pública no servidor remoto se ainda não estiver presente
func (s *SSHConnection) installPublicKeyIfNeeded(client *ssh.Client) error {
	// Se não há chave SSH configurada, não faz nada
	if len(s.SSHKeys) == 0 {
		s.debugLog("Instalação de chave pública: nenhuma chave SSH configurada")
		return nil
	}

	pubKeys := readConfiguredPublicKeys(s.SSHKeys, s.debugLog)
	if len(pubKeys) == 0 {
		s.debugLog("Nenhuma chave pública disponível para instalação")
		return nil
	}

	s.debugLog("Verificando instalação de %d chave(s) pública(s) no servidor...", len(pubKeys))
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("erro ao criar cliente SFTP para instalação de chave: %w", err)
	}
	defer sftpClient.Close()

	homeDir, err := sftpClient.Getwd()
	if err != nil {
		return fmt.Errorf("erro ao obter diretório home remoto: %w", err)
	}

	sshDir := path.Join(homeDir, ".ssh")
	authorizedKeysPath := path.Join(sshDir, "authorized_keys")

	if err := sftpClient.MkdirAll(sshDir); err != nil {
		return fmt.Errorf("erro ao criar diretório remoto .ssh: %w", err)
	}
	if err := sftpClient.Chmod(sshDir, 0700); err != nil {
		s.debugLog("Não foi possível ajustar permissão de %s: %v", sshDir, err)
	}

	existingKeys := make(map[string]struct{})
	if remoteFile, err := sftpClient.Open(authorizedKeysPath); err == nil {
		defer remoteFile.Close()

		content, readErr := io.ReadAll(remoteFile)
		if readErr != nil {
			return fmt.Errorf("erro ao ler authorized_keys remoto: %w", readErr)
		}

		for _, line := range strings.Split(string(content), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				existingKeys[line] = struct{}{}
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("erro ao abrir authorized_keys remoto: %w", err)
	}

	var missingKeys []string
	for _, pubKey := range pubKeys {
		if _, exists := existingKeys[pubKey]; exists {
			continue
		}
		missingKeys = append(missingKeys, pubKey)
	}

	if len(missingKeys) == 0 {
		s.debugLog("Todas as chaves públicas configuradas já estão instaladas no servidor")
		return nil
	}

	s.debugLog("Instalando %d chave(s) pública(s) ausente(s)...", len(missingKeys))
	remoteFile, err := sftpClient.OpenFile(authorizedKeysPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND)
	if err != nil {
		return fmt.Errorf("erro ao abrir authorized_keys para escrita: %w", err)
	}
	defer remoteFile.Close()

	if len(existingKeys) > 0 {
		if _, err := remoteFile.Write([]byte("\n")); err != nil {
			return fmt.Errorf("erro ao preparar separador em authorized_keys: %w", err)
		}
	}

	for _, pubKey := range missingKeys {
		if _, err := remoteFile.Write([]byte(pubKey + "\n")); err != nil {
			return fmt.Errorf("erro ao adicionar chave pública em authorized_keys: %w", err)
		}
	}

	if err := sftpClient.Chmod(authorizedKeysPath, 0600); err != nil {
		s.debugLog("Não foi possível ajustar permissão de %s: %v", authorizedKeysPath, err)
	}

	s.debugLog("%d chave(s) pública(s) instalada(s) com sucesso", len(missingKeys))
	fmt.Fprintf(os.Stderr, "✅ %d chave(s) pública(s) instalada(s) com sucesso no servidor remoto\n", len(missingKeys))
	return nil
}

// NewSSHConnection cria uma nova conexão SSH
func NewSSHConnection(user, host string, port int, sshKeys []string, password string, jumpHost *config.JumpHost, jumpHostSSHKeys []string, command string, proxyEnabled bool, proxyAddress string, proxyPort int, verbose bool) *SSHConnection {
	return &SSHConnection{
		User:                       user,
		Host:                       host,
		Port:                       port,
		SSHKeys:                    sshKeys,
		Password:                   password,
		JumpHost:                   jumpHost,
		JumpHostSSHKeys:            jumpHostSSHKeys,
		Command:                    command,
		ProxyEnabled:               proxyEnabled,
		ProxyAddress:               proxyAddress,
		ProxyPort:                  proxyPort,
		InteractivePasswordAllowed: true, // Por padrão, permite senha interativa (modo single host)
		Verbose:                    verbose,
	}
}

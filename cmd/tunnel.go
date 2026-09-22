package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	// DefaultSOCKSPort é a porta local padrão do proxy SOCKS5
	DefaultSOCKSPort = 4000

	// TunnelDaemonEnv sinaliza que o processo atual é o tunnel rodando em background
	TunnelDaemonEnv = "SC_TUNNEL_DAEMON"
	// tunnelPasswordStdinEnv indica que a senha será enviada pelo stdin ao processo em background
	tunnelPasswordStdinEnv = "SC_TUNNEL_PASSWORD_STDIN"

	// tunnelReadyFD é o descritor usado pelo processo em background para informar se o tunnel abriu
	tunnelReadyFD = 3

	tunnelStartTimeout      = 60 * time.Second
	tunnelHandshakeTimeout  = 30 * time.Second
	tunnelKeepAliveInterval = 30 * time.Second
)

var tunnelNameSanitizer = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// TunnelSession gerencia um tunnel SSH que expõe um proxy SOCKS5 local
type TunnelSession struct {
	SSHConn       *SSHConnection
	Name          string // Nome do jump host usado no tunnel
	LocalPort     int
	LogTraffic    bool          // Exibe cada conexão que passa pelo proxy (modo foreground)
	Routes        *TunnelRoutes // Rotas criadas antes do tunnel para alcançar o jump host (opcional)
	client        *ssh.Client
	listener      net.Listener
	activeConns   int64
	totalConns    int64
	bytesSent     int64
	bytesReceived int64
}

// NewTunnelSession cria uma nova sessão de tunnel SOCKS5
func NewTunnelSession(sshConn *SSHConnection, name string, localPort int) *TunnelSession {
	return &TunnelSession{
		SSHConn:   sshConn,
		Name:      name,
		LocalPort: localPort,
	}
}

// localAddress retorna o endereço local do proxy SOCKS5
func (t *TunnelSession) localAddress() string {
	return fmt.Sprintf("localhost:%d", t.LocalPort)
}

// open estabelece a conexão SSH e abre a porta local do proxy
func (t *TunnelSession) open() error {
	config, err := t.SSHConn.createSSHConfig()
	if err != nil {
		return fmt.Errorf("erro ao criar configuração SSH: %w", err)
	}

	client, err := t.SSHConn.dial(config)
	if err != nil {
		return fmt.Errorf("erro ao conectar em %s@%s:%d: %w", t.SSHConn.User, t.SSHConn.Host, t.SSHConn.Port, err)
	}
	t.SSHConn.debugLog("Conexão SSH estabelecida para o tunnel")

	// Tenta instalar a chave pública se necessário (não bloqueia em caso de erro)
	_ = t.SSHConn.installPublicKeyIfNeeded(client)

	// Escuta apenas em localhost para não expor o proxy na rede
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", t.LocalPort))
	if err != nil {
		client.Close()
		return fmt.Errorf("erro ao escutar na porta local %d: %w", t.LocalPort, err)
	}

	t.client = client
	t.listener = listener
	return nil
}

// serve aceita conexões SOCKS5 até receber sinal de término ou perder a conexão SSH.
// Retorna o motivo do encerramento e se ele foi causado por um sinal.
func (t *TunnelSession) serve() (string, bool) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigChan)

	lost := make(chan error, 1)
	go func() { lost <- t.client.Wait() }()
	go t.keepAlive()
	go t.acceptConnections()

	var reason string
	bySignal := false
	select {
	case sig := <-sigChan:
		reason = fmt.Sprintf("sinal recebido (%s)", sig)
		bySignal = true
	case err := <-lost:
		reason = "conexão SSH perdida"
		if err != nil {
			reason += fmt.Sprintf(": %v", err)
		}
	}

	t.listener.Close()
	t.client.Close()
	return reason, bySignal
}

// keepAlive envia requisições periódicas para manter a conexão SSH ativa
func (t *TunnelSession) keepAlive() {
	ticker := time.NewTicker(tunnelKeepAliveInterval)
	defer ticker.Stop()

	for range ticker.C {
		if _, _, err := t.client.SendRequest("keepalive@openssh.com", true, nil); err != nil {
			// Fechar o client faz o Wait() retornar e encerra o tunnel
			t.client.Close()
			return
		}
	}
}

// acceptConnections aceita conexões no listener local até ele ser fechado
func (t *TunnelSession) acceptConnections() {
	for {
		conn, err := t.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			tunnelLog("⚠️  Erro ao aceitar conexão: %v", err)
			continue
		}

		connNum := atomic.AddInt64(&t.totalConns, 1)
		go t.handleConnection(conn, connNum)
	}
}

// handleConnection negocia o SOCKS5 e repassa o tráfego pelo tunnel SSH
func (t *TunnelSession) handleConnection(localConn net.Conn, connNum int64) {
	defer localConn.Close()
	atomic.AddInt64(&t.activeConns, 1)
	defer atomic.AddInt64(&t.activeConns, -1)

	localConn.SetDeadline(time.Now().Add(tunnelHandshakeTimeout))
	target, remoteConn, err := socks5Handshake(localConn, t.client.Dial)
	if err != nil {
		if t.LogTraffic {
			if target == "" {
				target = "?"
			}
			tunnelLog("#%d ❌ %s → %s: %v", connNum, localConn.RemoteAddr(), target, err)
		}
		return
	}
	defer remoteConn.Close()
	localConn.SetDeadline(time.Time{})

	if t.LogTraffic {
		tunnelLog("#%d ✅ %s → %s", connNum, localConn.RemoteAddr(), target)
	}

	start := time.Now()
	sent, received := relay(localConn, remoteConn)
	atomic.AddInt64(&t.bytesSent, sent)
	atomic.AddInt64(&t.bytesReceived, received)

	if t.LogTraffic {
		tunnelLog("#%d 🔚 %s (↑%s ↓%s, %s)", connNum, target, formatBytes(sent), formatBytes(received), time.Since(start).Round(time.Millisecond))
	}
}

// relay copia dados nas duas direções até uma delas encerrar e retorna os bytes enviados e recebidos
func relay(local, remote net.Conn) (int64, int64) {
	var sent, received int64
	done := make(chan struct{}, 2)

	go func() {
		sent, _ = io.Copy(remote, local)
		done <- struct{}{}
	}()
	go func() {
		received, _ = io.Copy(local, remote)
		done <- struct{}{}
	}()

	// Ao terminar uma direção, fecha ambas para liberar a outra
	<-done
	local.Close()
	remote.Close()
	<-done

	return sent, received
}

// printStats exibe as estatísticas da sessão
func (t *TunnelSession) printStats() {
	fmt.Printf("📊 Estatísticas da sessão:\n")
	fmt.Printf("   Total de conexões: %d\n", atomic.LoadInt64(&t.totalConns))
	fmt.Printf("   Bytes enviados:    %s\n", formatBytes(atomic.LoadInt64(&t.bytesSent)))
	fmt.Printf("   Bytes recebidos:   %s\n", formatBytes(atomic.LoadInt64(&t.bytesReceived)))
}

// tunnelLog imprime uma linha de log com horário
func tunnelLog(format string, args ...interface{}) {
	fmt.Printf("[%s] %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
}

// RunForeground executa o tunnel no terminal atual exibindo o tráfego até Ctrl+C
func (t *TunnelSession) RunForeground(stateDir string) error {
	t.LogTraffic = true

	// Evita mexer nas rotas de um tunnel em background do mesmo jump host
	if info, ok := FindRunningTunnel(stateDir, t.Name); ok {
		return fmt.Errorf("já existe um tunnel ativo via %s em localhost:%d (PID %d). Use 'sc tunnel stop -j %s' para encerrá-lo", t.Name, info.Port, info.PID, t.Name)
	}

	fmt.Println()
	if err := setupTunnelRoutes(stateDir, t.Name, t.Routes); err != nil {
		return err
	}
	defer func() {
		if err := cleanupTunnelRoutes(stateDir, t.Name, true); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  %v\n", err)
		}
	}()

	fmt.Println("🔗 Conectando...")
	fmt.Printf("   %s\n", t.SSHConn.formatConnectionString())
	fmt.Println()

	if err := t.open(); err != nil {
		return err
	}

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("🧦 Tunnel SOCKS5 aberto via %s\n", t.Name)
	fmt.Printf("   Configure o proxy SOCKS5 no navegador: %s\n", t.localAddress())
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
	fmt.Println("Pressione Ctrl+C para encerrar...")
	fmt.Println()
	fmt.Println("📋 Tráfego:")
	fmt.Println("────────────────────────────────────────────────────────────────")

	reason, _ := t.serve()

	fmt.Println()
	fmt.Println("────────────────────────────────────────────────────────────────")
	t.printStats()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("🛑 Tunnel encerrado: %s\n", reason)
	return nil
}

// RunDaemon executa o tunnel como processo em background (iniciado por StartBackground).
// O resultado da abertura é informado ao processo pai pelo descritor tunnelReadyFD.
func (t *TunnelSession) RunDaemon(stateDir string) error {
	ready := os.NewFile(tunnelReadyFD, "tunnel-ready")
	report := func(msg string) {
		if ready != nil {
			fmt.Fprintln(ready, msg)
			ready.Close()
			ready = nil
		}
	}

	tunnelLog("Iniciando tunnel via %s (%s@%s:%d) na porta %d", t.Name, t.SSHConn.User, t.SSHConn.Host, t.SSHConn.Port, t.LocalPort)

	if err := t.open(); err != nil {
		tunnelLog("❌ %v", err)
		report("ERR " + strings.ReplaceAll(err.Error(), "\n", " "))
		return err
	}

	pidPath := tunnelPidPath(stateDir, t.Name)
	if err := writeTunnelPidFile(pidPath, TunnelInfo{Name: t.Name, PID: os.Getpid(), Port: t.LocalPort}); err != nil {
		t.listener.Close()
		t.client.Close()
		tunnelLog("❌ %v", err)
		report("ERR " + err.Error())
		return err
	}
	defer os.Remove(pidPath)

	tunnelLog("✅ Tunnel aberto em %s", t.localAddress())
	report("OK")

	reason, bySignal := t.serve()
	tunnelLog("🛑 Tunnel encerrado: %s (conexões: %d, ↑%s ↓%s)", reason,
		atomic.LoadInt64(&t.totalConns),
		formatBytes(atomic.LoadInt64(&t.bytesSent)),
		formatBytes(atomic.LoadInt64(&t.bytesReceived)))

	// Encerrado pelo 'sc tunnel stop': as rotas são removidas por ele, no terminal do usuário.
	// Encerrado sozinho (conexão perdida): tenta remover sem solicitar senha (sudo -n); se não for
	// possível, elas permanecem registradas e são removidas pelo 'sc tunnel stop' ou no próximo início.
	if !bySignal && loadTunnelRoutes(stateDir, t.Name) != nil {
		if err := cleanupTunnelRoutes(stateDir, t.Name, false); err != nil {
			tunnelLog("⚠️  %v", err)
		} else {
			tunnelLog("🧹 Rotas removidas")
		}
	}
	return nil
}

// ReadDaemonPassword lê a senha enviada pelo processo pai, se houver
func ReadDaemonPassword() (string, error) {
	if os.Getenv(tunnelPasswordStdinEnv) != "1" {
		return "", nil
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("erro ao ler senha do processo pai: %w", err)
	}
	return strings.TrimSuffix(line, "\n"), nil
}

// StartBackground inicia o tunnel em um processo desacoplado do terminal e aguarda sua abertura.
// As rotas configuradas são criadas antes (com sudo, se necessário) e removidas se o tunnel não abrir.
// args são os argumentos originais da linha de comando, repassados ao novo processo.
func (t *TunnelSession) StartBackground(stateDir string, args []string) error {
	if info, ok := FindRunningTunnel(stateDir, t.Name); ok {
		return fmt.Errorf("já existe um tunnel ativo via %s em localhost:%d (PID %d). Use 'sc tunnel stop -j %s' para encerrá-lo", t.Name, info.Port, info.PID, t.Name)
	}

	fmt.Println()
	if err := setupTunnelRoutes(stateDir, t.Name, t.Routes); err != nil {
		return err
	}

	if err := t.startDaemon(stateDir, args); err != nil {
		if cleanupErr := cleanupTunnelRoutes(stateDir, t.Name, true); cleanupErr != nil {
			fmt.Fprintf(os.Stderr, "⚠️  %v\n", cleanupErr)
		}
		return err
	}
	return nil
}

// startDaemon inicia o processo em background e aguarda a confirmação de abertura do tunnel
func (t *TunnelSession) startDaemon(stateDir string, args []string) error {
	name := t.Name
	password := t.SSHConn.Password

	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return fmt.Errorf("erro ao criar diretório %s: %w", stateDir, err)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("erro ao localizar executável: %w", err)
	}

	logPath := tunnelLogPath(stateDir, name)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("erro ao criar log %s: %w", logPath, err)
	}
	defer logFile.Close()

	readyR, readyW, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("erro ao criar pipe: %w", err)
	}
	defer readyR.Close()

	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		readyW.Close()
		return fmt.Errorf("erro ao criar pipe: %w", err)
	}

	daemon := exec.Command(exe, args...)
	daemon.Env = append(os.Environ(), TunnelDaemonEnv+"=1")
	if password != "" {
		daemon.Env = append(daemon.Env, tunnelPasswordStdinEnv+"=1")
	}
	daemon.Stdin = stdinR
	daemon.Stdout = logFile
	daemon.Stderr = logFile
	daemon.ExtraFiles = []*os.File{readyW} // vira o descritor 3 no processo filho
	daemon.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	err = daemon.Start()
	stdinR.Close()
	readyW.Close()
	if err != nil {
		stdinW.Close()
		return fmt.Errorf("erro ao iniciar tunnel em background: %w", err)
	}

	if password != "" {
		fmt.Fprintln(stdinW, password)
	}
	stdinW.Close()

	pid := daemon.Process.Pid
	daemon.Process.Release()

	// Aguarda o processo em background informar se o tunnel abriu
	result := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(readyR).ReadString('\n')
		result <- strings.TrimSpace(line)
	}()

	fmt.Println("🔗 Conectando...")
	fmt.Printf("   %s\n", t.SSHConn.formatConnectionString())

	var status string
	select {
	case status = <-result:
	case <-time.After(tunnelStartTimeout):
		syscall.Kill(pid, syscall.SIGTERM)
		return fmt.Errorf("tempo esgotado aguardando o tunnel abrir (veja %s)", logPath)
	}

	switch {
	case status == "OK":
	case strings.HasPrefix(status, "ERR "):
		return errors.New(strings.TrimPrefix(status, "ERR "))
	default:
		return fmt.Errorf("o processo do tunnel encerrou inesperadamente (veja %s)", logPath)
	}

	address := t.localAddress()
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("🧦 Tunnel SOCKS5 aberto em background via %s\n", name)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("   Configure o proxy SOCKS5 no navegador: %s\n", address)
	fmt.Printf("   Teste via terminal: curl --socks5-hostname %s https://ifconfig.me\n", address)
	fmt.Println()
	fmt.Printf("   PID: %d | Log: %s\n", pid, logPath)
	fmt.Printf("   Para encerrar: sc tunnel stop -j %s\n", name)
	fmt.Println()
	return nil
}

// TunnelInfo representa um tunnel em execução em background
type TunnelInfo struct {
	Name string
	PID  int
	Port int
}

// TunnelStateDir retorna o diretório onde ficam os arquivos de PID e log dos tunnels
func TunnelStateDir(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "tunnels")
}

func tunnelFileBase(stateDir, name string) string {
	return filepath.Join(stateDir, tunnelNameSanitizer.ReplaceAllString(name, "_"))
}

func tunnelPidPath(stateDir, name string) string {
	return tunnelFileBase(stateDir, name) + ".pid"
}

func tunnelLogPath(stateDir, name string) string {
	return tunnelFileBase(stateDir, name) + ".log"
}

// writeTunnelPidFile grava o arquivo de estado no formato "pid porta nome"
func writeTunnelPidFile(path string, info TunnelInfo) error {
	content := fmt.Sprintf("%d %d %s\n", info.PID, info.Port, info.Name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return fmt.Errorf("erro ao gravar arquivo de PID %s: %w", path, err)
	}
	return nil
}

// readTunnelPidFile lê o arquivo de estado de um tunnel
func readTunnelPidFile(path string) (TunnelInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TunnelInfo{}, err
	}

	fields := strings.SplitN(strings.TrimSpace(string(data)), " ", 3)
	if len(fields) < 2 {
		return TunnelInfo{}, fmt.Errorf("arquivo de PID inválido: %s", path)
	}

	pid, err := strconv.Atoi(fields[0])
	if err != nil {
		return TunnelInfo{}, fmt.Errorf("PID inválido em %s", path)
	}
	port, err := strconv.Atoi(fields[1])
	if err != nil {
		return TunnelInfo{}, fmt.Errorf("porta inválida em %s", path)
	}

	name := strings.TrimSuffix(filepath.Base(path), ".pid")
	if len(fields) >= 3 {
		name = fields[2]
	}

	return TunnelInfo{Name: name, PID: pid, Port: port}, nil
}

// processAlive verifica se o processo ainda está em execução
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// tunnelAlive verifica se o processo existe e se a porta do proxy está aceitando conexões.
// A checagem da porta evita tratar como tunnel um PID reaproveitado por outro processo.
func tunnelAlive(info TunnelInfo) bool {
	if !processAlive(info.PID) {
		return false
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", info.Port), time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// FindRunningTunnel retorna o tunnel ativo para o jump host informado, removendo arquivos órfãos
func FindRunningTunnel(stateDir, name string) (TunnelInfo, bool) {
	path := tunnelPidPath(stateDir, name)
	info, err := readTunnelPidFile(path)
	if err != nil {
		return TunnelInfo{}, false
	}
	if !tunnelAlive(info) {
		os.Remove(path)
		return TunnelInfo{}, false
	}
	return info, true
}

// ListRunningTunnels retorna todos os tunnels ativos, removendo arquivos órfãos
func ListRunningTunnels(stateDir string) []TunnelInfo {
	paths, _ := filepath.Glob(filepath.Join(stateDir, "*.pid"))
	sort.Strings(paths)

	var tunnels []TunnelInfo
	for _, path := range paths {
		info, err := readTunnelPidFile(path)
		if err != nil || !tunnelAlive(info) {
			os.Remove(path)
			continue
		}
		tunnels = append(tunnels, info)
	}
	return tunnels
}

// PrintTunnelStatus exibe os tunnels ativos e rotas pendentes de tunnels encerrados
func PrintTunnelStatus(stateDir string) {
	tunnels := ListRunningTunnels(stateDir)
	pending := pendingTunnelRoutes(stateDir, "", tunnels)

	fmt.Println()
	for _, routes := range pending {
		fmt.Printf("⚠️  Rotas pendentes do tunnel via %s: %s via %s (execute 'sc tunnel stop -j %s')\n",
			routes.Name, strings.Join(routes.Networks, ", "), routes.Gateway, routes.Name)
	}
	if len(pending) > 0 {
		fmt.Println()
	}

	if len(tunnels) == 0 {
		fmt.Println("ℹ️  Nenhum tunnel ativo")
		fmt.Println()
		return
	}

	fmt.Println("🧦 Tunnels SOCKS5 ativos:")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("%-20s %-20s %-8s %s\n", "Jump Host", "Proxy SOCKS5", "PID", "Rotas")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	for _, tn := range tunnels {
		routes := "-"
		if r := loadTunnelRoutes(stateDir, tn.Name); r != nil {
			routes = fmt.Sprintf("%s via %s", strings.Join(r.Networks, ", "), r.Gateway)
		}
		fmt.Printf("%-20s %-20s %-8d %s\n", tn.Name, fmt.Sprintf("localhost:%d", tn.Port), tn.PID, routes)
	}
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Total: %d tunnel(s)\n", len(tunnels))
	fmt.Println()
}

// StopTunnels encerra o tunnel do jump host informado, ou todos se name for vazio.
// Também remove as rotas criadas pelos tunnels, inclusive de tunnels que já encerraram.
func StopTunnels(stateDir, name string) error {
	var tunnels []TunnelInfo
	if name != "" {
		if info, ok := FindRunningTunnel(stateDir, name); ok {
			tunnels = []TunnelInfo{info}
		}
	} else {
		tunnels = ListRunningTunnels(stateDir)
	}
	pending := pendingTunnelRoutes(stateDir, name, tunnels)

	if len(tunnels) == 0 && len(pending) == 0 {
		if name != "" {
			return fmt.Errorf("nenhum tunnel ativo via %s", name)
		}
		return fmt.Errorf("nenhum tunnel ativo")
	}

	var failed []string
	for _, tn := range tunnels {
		if err := syscall.Kill(tn.PID, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
			failed = append(failed, fmt.Sprintf("%s (PID %d): %v", tn.Name, tn.PID, err))
			continue
		}

		// Aguarda o processo encerrar e remover o próprio arquivo de PID
		deadline := time.Now().Add(5 * time.Second)
		for processAlive(tn.PID) && time.Now().Before(deadline) {
			time.Sleep(100 * time.Millisecond)
		}
		if processAlive(tn.PID) {
			failed = append(failed, fmt.Sprintf("%s (PID %d): processo não encerrou", tn.Name, tn.PID))
			continue
		}
		os.Remove(tunnelPidPath(stateDir, tn.Name))
		fmt.Printf("🛑 Tunnel via %s (localhost:%d) encerrado\n", tn.Name, tn.Port)

		if err := cleanupTunnelRoutes(stateDir, tn.Name, true); err != nil {
			failed = append(failed, err.Error())
		}
	}

	// Rotas de tunnels que encerraram sozinhos (ex: conexão perdida)
	for _, routes := range pending {
		if err := cleanupTunnelRoutes(stateDir, routes.Name, true); err != nil {
			failed = append(failed, err.Error())
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("falha ao encerrar: %s", strings.Join(failed, "; "))
	}
	return nil
}

package cmd

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
)

// PortForward representa uma configuração de port forwarding
type PortForward struct {
	LocalPort  int
	RemoteHost string
	RemotePort int
}

// PortForwardSession gerencia uma sessão de port forwarding
type PortForwardSession struct {
	SSHConn       *SSHConnection
	Forward       PortForward
	listener      net.Listener
	client        *ssh.Client
	activeConns   int64
	totalConns    int64
	bytesReceived int64
	bytesSent     int64
	mu            sync.Mutex
	done          chan struct{}
}

// NewPortForwardSession cria uma nova sessão de port forwarding
func NewPortForwardSession(sshConn *SSHConnection, forward PortForward) *PortForwardSession {
	return &PortForwardSession{
		SSHConn: sshConn,
		Forward: forward,
		done:    make(chan struct{}),
	}
}

// Start inicia o port forwarding
func (pf *PortForwardSession) Start() error {
	// Exibe informações de conexão
	fmt.Println()
	fmt.Println("🔗 Conectando...")
	fmt.Printf("   %s\n", pf.SSHConn.formatConnectionString())
	fmt.Println()

	// Cria a configuração SSH
	config, err := pf.SSHConn.createSSHConfig()
	if err != nil {
		return fmt.Errorf("erro ao criar configuração SSH: %w", err)
	}

	// Conecta ao host (via Jump Host se necessário)
	client, err := pf.SSHConn.dial(config)
	if err != nil {
		return fmt.Errorf("erro ao conectar: %w", err)
	}
	pf.client = client

	// Inicia listener local
	localAddr := fmt.Sprintf("0.0.0.0:%d", pf.Forward.LocalPort)
	listener, err := net.Listen("tcp", localAddr)
	if err != nil {
		client.Close()
		return fmt.Errorf("erro ao escutar na porta local %d: %w", pf.Forward.LocalPort, err)
	}
	pf.listener = listener

	// Exibe informações do tunnel
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("🚇 Port Forward Ativo\n")
	fmt.Printf("   Local:  0.0.0.0:%d\n", pf.Forward.LocalPort)
	fmt.Printf("   Remoto: %s:%d (via %s)\n", pf.Forward.RemoteHost, pf.Forward.RemotePort, pf.SSHConn.Host)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
	fmt.Println("Pressione Ctrl+C para encerrar...")
	fmt.Println()
	fmt.Println("📋 Log de conexões:")
	fmt.Println("────────────────────────────────────────────────────────────────")

	// Configura handler para Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Goroutine para aceitar conexões
	go pf.acceptConnections()

	// Aguarda sinal de interrupção
	<-sigChan

	// Encerra
	close(pf.done)
	pf.Stop()

	return nil
}

// acceptConnections aceita novas conexões no listener local
func (pf *PortForwardSession) acceptConnections() {
	for {
		select {
		case <-pf.done:
			return
		default:
		}

		// Define timeout para não bloquear indefinidamente
		pf.listener.(*net.TCPListener).SetDeadline(time.Now().Add(1 * time.Second))

		conn, err := pf.listener.Accept()
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
				continue
			}
			select {
			case <-pf.done:
				return
			default:
				fmt.Fprintf(os.Stderr, "⚠️  Erro ao aceitar conexão: %v\n", err)
				continue
			}
		}

		// Nova conexão recebida
		atomic.AddInt64(&pf.totalConns, 1)
		atomic.AddInt64(&pf.activeConns, 1)
		connNum := atomic.LoadInt64(&pf.totalConns)

		timestamp := time.Now().Format("15:04:05")
		fmt.Printf("[%s] #%d ✅ Conexão de %s\n", timestamp, connNum, conn.RemoteAddr().String())

		go pf.handleConnection(conn, connNum)
	}
}

// handleConnection gerencia uma conexão individual
func (pf *PortForwardSession) handleConnection(localConn net.Conn, connNum int64) {
	defer func() {
		localConn.Close()
		atomic.AddInt64(&pf.activeConns, -1)
	}()

	// Conecta ao destino remoto via SSH
	remoteAddr := fmt.Sprintf("%s:%d", pf.Forward.RemoteHost, pf.Forward.RemotePort)
	remoteConn, err := pf.client.Dial("tcp", remoteAddr)
	if err != nil {
		timestamp := time.Now().Format("15:04:05")
		fmt.Printf("[%s] #%d ❌ Erro ao conectar ao remoto: %v\n", timestamp, connNum, err)
		return
	}
	defer remoteConn.Close()

	// Canais para sinalizar término
	done := make(chan struct{}, 2)
	var sent, received int64

	// Copia dados bidirecional com contagem de bytes
	go func() {
		n, _ := io.Copy(remoteConn, localConn)
		atomic.AddInt64(&sent, n)
		atomic.AddInt64(&pf.bytesSent, n)
		done <- struct{}{}
	}()

	go func() {
		n, _ := io.Copy(localConn, remoteConn)
		atomic.AddInt64(&received, n)
		atomic.AddInt64(&pf.bytesReceived, n)
		done <- struct{}{}
	}()

	// Aguarda término de uma das direções
	<-done

	// Fecha conexões para forçar término da outra direção
	localConn.Close()
	remoteConn.Close()

	// Aguarda a outra direção terminar
	<-done

	// Log de encerramento
	timestamp := time.Now().Format("15:04:05")
	fmt.Printf("[%s] #%d 🔚 Encerrada (↑%s ↓%s)\n",
		timestamp, connNum,
		formatBytes(atomic.LoadInt64(&sent)),
		formatBytes(atomic.LoadInt64(&received)))
}

// Stop encerra a sessão de port forwarding
func (pf *PortForwardSession) Stop() {
	fmt.Println()
	fmt.Println("────────────────────────────────────────────────────────────────")
	fmt.Printf("📊 Estatísticas da sessão:\n")
	fmt.Printf("   Total de conexões: %d\n", atomic.LoadInt64(&pf.totalConns))
	fmt.Printf("   Bytes enviados:    %s\n", formatBytes(atomic.LoadInt64(&pf.bytesSent)))
	fmt.Printf("   Bytes recebidos:   %s\n", formatBytes(atomic.LoadInt64(&pf.bytesReceived)))
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
	fmt.Println("🛑 Port forward encerrado.")

	if pf.listener != nil {
		pf.listener.Close()
	}
	if pf.client != nil {
		pf.client.Close()
	}
}


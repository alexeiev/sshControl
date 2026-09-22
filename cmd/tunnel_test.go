package cmd

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// socks5Connect executa o lado cliente do SOCKS5 (sem autenticação, CONNECT por domínio)
func socks5Connect(t *testing.T, conn net.Conn, host string, port int) byte {
	t.Helper()

	if _, err := conn.Write([]byte{socks5Version, 1, socks5AuthNone}); err != nil {
		t.Fatalf("erro ao enviar saudação: %v", err)
	}
	greeting := make([]byte, 2)
	if _, err := io.ReadFull(conn, greeting); err != nil {
		t.Fatalf("erro ao ler resposta da saudação: %v", err)
	}
	if greeting[1] != socks5AuthNone {
		t.Fatalf("método de autenticação = %d, want %d", greeting[1], socks5AuthNone)
	}

	req := []byte{socks5Version, socks5CmdConnect, 0x00, socks5AtypDomain, byte(len(host))}
	req = append(req, host...)
	req = binary.BigEndian.AppendUint16(req, uint16(port))
	if _, err := conn.Write(req); err != nil {
		t.Fatalf("erro ao enviar requisição: %v", err)
	}

	reply := make([]byte, 10)
	if _, err := io.ReadFull(conn, reply); err != nil {
		t.Fatalf("erro ao ler resposta da requisição: %v", err)
	}
	return reply[1]
}

func TestSocks5HandshakeConnect(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	var dialed string
	remoteSide, remoteOther := net.Pipe()
	defer remoteOther.Close()
	dial := func(network, address string) (net.Conn, error) {
		dialed = address
		return remoteSide, nil
	}

	type result struct {
		target string
		remote net.Conn
		err    error
	}
	done := make(chan result, 1)
	go func() {
		target, remote, err := socks5Handshake(server, dial)
		done <- result{target, remote, err}
	}()

	if rep := socks5Connect(t, client, "intranet.local", 8080); rep != socks5RepSuccess {
		t.Fatalf("resposta SOCKS = %d, want sucesso", rep)
	}

	res := <-done
	if res.err != nil {
		t.Fatalf("socks5Handshake retornou erro: %v", res.err)
	}
	if res.target != "intranet.local:8080" || dialed != "intranet.local:8080" {
		t.Fatalf("destino = %q (dial %q), want intranet.local:8080", res.target, dialed)
	}
	if res.remote != remoteSide {
		t.Fatalf("conexão remota incorreta")
	}
}

func TestSocks5HandshakeDialFailure(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	dial := func(network, address string) (net.Conn, error) {
		return nil, errors.New("connection refused")
	}

	done := make(chan error, 1)
	go func() {
		_, _, err := socks5Handshake(server, dial)
		done <- err
	}()

	if rep := socks5Connect(t, client, "10.0.0.1", 22); rep != socks5RepGeneralFailure {
		t.Fatalf("resposta SOCKS = %d, want falha geral", rep)
	}
	if err := <-done; err == nil {
		t.Fatalf("socks5Handshake deveria retornar erro")
	}
}

func TestSocks5HandshakeRejectsAuthRequired(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	done := make(chan error, 1)
	go func() {
		_, _, err := socks5Handshake(server, nil)
		done <- err
	}()

	// Cliente oferece apenas usuário/senha (0x02)
	client.Write([]byte{socks5Version, 1, 0x02})
	reply := make([]byte, 2)
	io.ReadFull(client, reply)
	if reply[1] != socks5AuthNoAcceptable {
		t.Fatalf("resposta = %d, want %d", reply[1], socks5AuthNoAcceptable)
	}
	if err := <-done; err == nil {
		t.Fatalf("socks5Handshake deveria retornar erro")
	}
}

func TestTunnelPidFileRoundTrip(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	path := tunnelPidPath(stateDir, "prod jump/1")
	if filepath.Dir(path) != stateDir || filepath.Base(path) != "prod_jump_1.pid" {
		t.Fatalf("caminho do PID inesperado: %s", path)
	}

	want := TunnelInfo{Name: "prod jump/1", PID: 4242, Port: 4000}
	if err := writeTunnelPidFile(path, want); err != nil {
		t.Fatalf("writeTunnelPidFile retornou erro: %v", err)
	}
	got, err := readTunnelPidFile(path)
	if err != nil {
		t.Fatalf("readTunnelPidFile retornou erro: %v", err)
	}
	if got != want {
		t.Fatalf("readTunnelPidFile = %+v, want %+v", got, want)
	}
}

func TestListRunningTunnelsRemovesStale(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()

	// PID do próprio teste com porta sem listener: deve ser tratado como órfão
	stale := tunnelPidPath(stateDir, "stale")
	writeTunnelPidFile(stale, TunnelInfo{Name: "stale", PID: os.Getpid(), Port: freePort(t)})

	// PID do próprio teste com listener ativo: deve ser listado
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("erro ao criar listener: %v", err)
	}
	defer ln.Close()
	activePort := ln.Addr().(*net.TCPAddr).Port
	writeTunnelPidFile(tunnelPidPath(stateDir, "active"), TunnelInfo{Name: "active", PID: os.Getpid(), Port: activePort})

	tunnels := ListRunningTunnels(stateDir)
	if len(tunnels) != 1 || tunnels[0].Name != "active" || tunnels[0].Port != activePort {
		t.Fatalf("ListRunningTunnels = %+v, want apenas 'active'", tunnels)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("arquivo de PID órfão não foi removido")
	}
}

func TestTunnelSessionEndToEnd(t *testing.T) {
	t.Parallel()

	// Servidor TCP de destino (echo) acessível apenas "pelo lado remoto"
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("erro ao criar servidor echo: %v", err)
	}
	defer echo.Close()
	go func() {
		for {
			c, err := echo.Accept()
			if err != nil {
				return
			}
			go func() { io.Copy(c, c); c.Close() }()
		}
	}()

	sshAddr := startTestSSHServer(t, "tunnel", "secret")
	host, portStr, _ := net.SplitHostPort(sshAddr)
	sshPort, _ := strconv.Atoi(portStr)

	sshConn := NewSSHConnection("tunnel", host, sshPort, nil, "secret", nil, nil, "", false, "", 0, false)
	sshConn.InteractivePasswordAllowed = false
	session := NewTunnelSession(sshConn, "test-jump", freePort(t))

	if err := session.open(); err != nil {
		t.Fatalf("open retornou erro: %v", err)
	}
	served := make(chan string, 1)
	go func() {
		reason, _ := session.serve()
		served <- reason
	}()

	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", session.LocalPort))
	if err != nil {
		t.Fatalf("erro ao conectar no proxy SOCKS5: %v", err)
	}
	echoPort := echo.Addr().(*net.TCPAddr).Port
	if rep := socks5Connect(t, conn, "127.0.0.1", echoPort); rep != socks5RepSuccess {
		t.Fatalf("resposta SOCKS = %d, want sucesso", rep)
	}

	conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("erro ao enviar dados pelo tunnel: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil || string(buf) != "ping" {
		t.Fatalf("resposta pelo tunnel = %q (%v), want ping", buf, err)
	}
	conn.Close()

	// Encerrar a conexão SSH deve finalizar o serve
	session.client.Close()
	select {
	case reason := <-served:
		if reason == "" {
			t.Fatalf("serve retornou sem motivo")
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("serve não encerrou após perda da conexão SSH")
	}
}

// freePort retorna uma porta TCP livre em localhost
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("erro ao obter porta livre: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// startTestSSHServer inicia um servidor SSH mínimo com suporte a direct-tcpip
func startTestSSHServer(t *testing.T, user, password string) string {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("erro ao gerar chave do servidor: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("erro ao criar signer: %v", err)
	}

	cfg := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if c.User() == user && string(pass) == password {
				return nil, nil
			}
			return nil, errors.New("senha incorreta")
		},
	}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("erro ao criar listener SSH: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			nConn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveTestSSHConn(nConn, cfg)
		}
	}()

	return ln.Addr().String()
}

func serveTestSSHConn(nConn net.Conn, cfg *ssh.ServerConfig) {
	_, chans, reqs, err := ssh.NewServerConn(nConn, cfg)
	if err != nil {
		nConn.Close()
		return
	}
	go ssh.DiscardRequests(reqs)

	for newChan := range chans {
		if newChan.ChannelType() != "direct-tcpip" {
			newChan.Reject(ssh.UnknownChannelType, "apenas direct-tcpip")
			continue
		}

		var payload struct {
			Host       string
			Port       uint32
			OriginHost string
			OriginPort uint32
		}
		if err := ssh.Unmarshal(newChan.ExtraData(), &payload); err != nil {
			newChan.Reject(ssh.ConnectionFailed, "payload inválido")
			continue
		}

		target, err := net.Dial("tcp", net.JoinHostPort(payload.Host, strconv.Itoa(int(payload.Port))))
		if err != nil {
			newChan.Reject(ssh.ConnectionFailed, err.Error())
			continue
		}

		ch, chReqs, err := newChan.Accept()
		if err != nil {
			target.Close()
			continue
		}
		go ssh.DiscardRequests(chReqs)
		go func() {
			go func() { io.Copy(ch, target); ch.CloseWrite() }()
			io.Copy(target, ch)
			target.Close()
			ch.Close()
		}()
	}
}

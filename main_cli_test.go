package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alexeiev/sshControl/config"
	"golang.org/x/crypto/ssh"
)

// Os handlers usam os.Exit em falhas; o subprocesso permite testar a CLI real
// sem encerrar o runner de testes nem acessar servidores externos.
func TestCLIHelperProcess(t *testing.T) {
	if os.Getenv("SSHCONTROL_TEST_CLI_PROCESS") != "1" {
		return
	}
	separator := slices.Index(os.Args, "--")
	if separator < 0 {
		t.Fatal("argumentos da CLI ausentes")
	}
	rootCmd.SetArgs(os.Args[separator+1:])
	main()
	os.Exit(0)
}

func TestSubcommandUserSelectionReachesSSH(t *testing.T) {
	for _, flow := range []string{"cp up", "cp down", "port-forward", "tunnel"} {
		for _, selection := range []struct {
			name, want               string
			flags                    []string
			explicit, alias, viaJump bool
		}{
			{name: "índice curto antes do comando", want: "selected-user", flags: []string{"-u1"}},
			{name: "índice curto com jump do exemplo", want: "selected-user", flags: []string{"-j2", "-u1"}, viaJump: true},
			{name: "nome longo antes do comando", want: "selected-user", flags: []string{"--user", "selected-user"}},
			{name: "alias do YAML", want: "selected-user", flags: []string{"-u", "1"}, alias: true},
			{name: "default_user preservado", want: "default-user"},
			{name: "usuário explícito preservado", want: "explicit-user", flags: []string{"-u1"}, explicit: true},
		} {
			if flow == "tunnel" && (selection.alias || selection.explicit) {
				continue
			}
			t.Run(flow+"/"+selection.name, func(t *testing.T) {
				address, attempts := startCLIAuthRecorder(t, selection.viaJump)
				host, portText, err := net.SplitHostPort(address)
				if err != nil {
					t.Fatal(err)
				}
				port, err := strconv.Atoi(portText)
				if err != nil {
					t.Fatal(err)
				}
				home := t.TempDir()
				cfg := &config.ConfigFile{Config: config.Config{
					DefaultUser: "default-user",
					User:        []config.User{{Name: "selected-user"}, {Name: "default-user"}, {Name: "explicit-user"}},
					JumpHosts:   []config.JumpHost{{Name: "unused", Host: "127.0.0.1", Port: 1, User: "unused"}, {Name: "jump", Host: host, Port: port, User: "jump-user"}},
				}, Hosts: []config.Host{{Name: "configured-server", Host: host, Port: port}}}
				configFile := filepath.Join(home, "config.yaml")
				if err := cfg.SaveConfig(configFile); err != nil {
					t.Fatal(err)
				}
				local := filepath.Join(home, "test.up")
				if err := os.WriteFile(local, []byte("teste"), 0600); err != nil {
					t.Fatal(err)
				}
				destination := address
				if selection.explicit {
					destination = "explicit-user@" + address
				}
				if selection.alias {
					destination = "configured-server"
				}
				args := append([]string{}, selection.flags...)
				args = append(args, "-P")
				want := selection.want
				switch flow {
				case "cp up":
					args = append(args, "cp", "up", local, destination)
				case "cp down":
					args = append(args, "cp", "down", destination, "/unused", home)
				case "port-forward":
					args = append(args, "port-forward", destination, "8080:80")
				case "tunnel":
					args = append(args, "-j2", "tunnel", "-v")
					if len(selection.flags) == 0 {
						want = "jump-user"
					}
				}
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				process := exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestCLIHelperProcess$", "--"}, args...)...)
				process.Env = append(os.Environ(), "SSHCONTROL_TEST_CLI_PROCESS=1", "SCCONFIG="+configFile, "HOME="+home, "SSH_AUTH_SOCK=", "SCPW=test-password", "GORACE=atexit_sleep_ms=0")
				output, err := process.CombinedOutput()
				if ctx.Err() != nil {
					t.Fatalf("CLI não encerrou: %s", output)
				}
				if err == nil {
					t.Fatal("o servidor de teste deveria recusar a autenticação após registrá-la")
				}
				select {
				case attempt := <-attempts:
					if attempt.user != want || attempt.password != "test-password" || (flow != "tunnel" && attempt.viaJump != selection.viaJump) {
						t.Fatalf("autenticação = %+v; esperado usuário %s e senha de SCPW; saída: %s", attempt, want, output)
					}
				default:
					t.Fatalf("CLI não chegou ao SSH: %s", output)
				}
				if strings.HasPrefix(flow, "cp ") && !strings.Contains(string(output), "❌ User: "+want+" Host:") {
					t.Fatalf("resultado não informa o usuário remoto utilizado: %s", output)
				}
				if flow == "tunnel" && !strings.Contains(string(output), "[DEBUG]") {
					t.Fatalf("-v sem efeito: %s", output)
				}
			})
		}
	}
}

type cliAuthAttempt struct {
	user, password string
	viaJump        bool
}

func startCLIAuthRecorder(t *testing.T, allowJump bool) (string, <-chan cliAuthAttempt) {
	t.Helper()
	attempts := make(chan cliAuthAttempt, 8)
	var jumpAccepted atomic.Bool
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{PasswordCallback: func(c ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if allowJump && c.User() == "jump-user" && string(password) == "test-password" {
			jumpAccepted.Store(true)
			return nil, nil
		}
		attempts <- cliAuthAttempt{c.User(), string(password), jumpAccepted.Load()}
		return nil, fmt.Errorf("autenticação recusada intencionalmente pelo teste")
	}}
	cfg.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				client, channels, requests, err := ssh.NewServerConn(conn, cfg)
				if err != nil {
					return
				}
				defer func() { _ = client.Close() }()
				go ssh.DiscardRequests(requests)
				for request := range channels {
					var destination struct {
						Host       string
						Port       uint32
						Origin     string
						OriginPort uint32
					}
					if request.ChannelType() != "direct-tcpip" || ssh.Unmarshal(request.ExtraData(), &destination) != nil {
						_ = request.Reject(ssh.UnknownChannelType, "apenas encaminhamento de teste")
						continue
					}
					remote, err := net.DialTimeout("tcp", net.JoinHostPort(destination.Host, strconv.Itoa(int(destination.Port))), 5*time.Second)
					if err != nil {
						_ = request.Reject(ssh.ConnectionFailed, err.Error())
						continue
					}
					stream, requests, err := request.Accept()
					if err != nil {
						_ = remote.Close()
						continue
					}
					go ssh.DiscardRequests(requests)
					go func() { _, _ = io.Copy(remote, stream); _ = remote.Close() }()
					go func() { _, _ = io.Copy(stream, remote); _ = stream.Close() }()
				}
			}()
		}
	}()
	return listener.Addr().String(), attempts
}

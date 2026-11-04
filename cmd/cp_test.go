package cmd

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexeiev/sshControl/config"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// O servidor e todos os arquivos de cada teste ficam em recursos temporários.
func startTestSFTPServer(t *testing.T, user string) (string, ssh.PublicKey, string) {
	t.Helper()
	root := t.TempDir()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{PasswordCallback: func(c ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if c.User() != user || string(password) != "secret" {
			return nil, fmt.Errorf("usuário ou senha incorretos")
		}
		return nil, nil
	}}
	cfg.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var active sync.Map
	t.Cleanup(func() {
		_ = listener.Close()
		active.Range(func(conn, _ any) bool { _ = conn.(net.Conn).Close(); return true })
	})
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			active.Store(conn, true)
			go func() {
				defer active.Delete(conn)
				defer func() { _ = conn.Close() }()
				_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
				client, channels, requests, err := ssh.NewServerConn(conn, cfg)
				if err != nil {
					return
				}
				defer func() { _ = client.Close() }()
				go ssh.DiscardRequests(requests)
				for channel := range channels {
					if channel.ChannelType() != "session" {
						_ = channel.Reject(ssh.UnknownChannelType, "apenas SFTP")
						continue
					}
					stream, requests, err := channel.Accept()
					if err != nil {
						return
					}
					go func() {
						defer func() { _ = stream.Close() }()
						for request := range requests {
							var payload struct{ Name string }
							if request.Type != "subsystem" || ssh.Unmarshal(request.Payload, &payload) != nil || payload.Name != "sftp" {
								_ = request.Reply(false, nil)
								continue
							}
							_ = request.Reply(true, nil)
							server, err := sftp.NewServer(stream, sftp.WithServerWorkingDirectory(root))
							if err != nil {
								return
							}
							_ = server.Serve()
							_ = server.Close()
							return
						}
					}()
				}
			}()
		}
	}()
	return listener.Addr().String(), signer.PublicKey(), root
}

func transferTestConnection(t *testing.T, address, knownHosts string, jump *config.JumpHost) *SSHConnection {
	t.Helper()
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	connection := NewSSHConnection("selected-user", host, port, nil, "secret", jump, nil, "", false, "", 0, false, HostKeyOptions{KnownHostsFile: knownHosts})
	connection.InteractivePasswordAllowed = false
	return connection
}

func assertTransferContent(t *testing.T, filename, expected string) {
	t.Helper()
	data, err := os.ReadFile(filename)
	if err != nil || string(data) != expected {
		t.Fatalf("conteúdo de %s = %q; erro = %v; esperado %q", filename, data, err, expected)
	}
}

func TestFileTransferUploadAndDownload(t *testing.T) {
	t.Parallel()
	for _, viaJump := range []bool{false, true} {
		t.Run(fmt.Sprintf("jump=%v", viaJump), func(t *testing.T) {
			address, key, remoteRoot := startTestSFTPServer(t, "selected-user")
			keys := map[string]ssh.PublicKey{address: key}
			var jumpHost *config.JumpHost
			if viaJump {
				jump, key := startTestSSHServer(t, "jump-user", "secret")
				keys[jump] = key
				host, portText, err := net.SplitHostPort(jump)
				if err != nil {
					t.Fatal(err)
				}
				port, err := strconv.Atoi(portText)
				if err != nil {
					t.Fatal(err)
				}
				jumpHost = &config.JumpHost{Host: host, Port: port, User: "jump-user"}
			}
			connection := transferTestConnection(t, address, trustedTestHostKeys(t, keys), jumpHost)
			for _, content := range []string{"arquivo de teste\n", ""} {
				local := filepath.Join(t.TempDir(), "test.up")
				if err := os.WriteFile(local, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
				transfer := &FileTransfer{LocalPath: local, RemotePath: "~"}
				if err := transfer.Upload(connection); err != nil {
					t.Fatal(err)
				}
				remote := filepath.Join(remoteRoot, "test.up")
				assertTransferContent(t, remote, content)
				transfer.LocalPath, transfer.RemotePath = t.TempDir(), remote
				if err := transfer.Download(connection); err != nil {
					t.Fatal(err)
				}
				assertTransferContent(t, filepath.Join(transfer.LocalPath, "test.up"), content)
			}
		})
	}
}

func TestFileTransferRecursiveAndMissingPaths(t *testing.T) {
	t.Parallel()
	address, key, remoteRoot := startTestSFTPServer(t, "selected-user")
	connection := transferTestConnection(t, address, trustedTestHostKeys(t, map[string]ssh.PublicKey{address: key}), nil)
	local := filepath.Join(t.TempDir(), "tree")
	if err := os.MkdirAll(filepath.Join(local, "nested", "deeper"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"top.txt", "nested/child.txt", "nested/deeper/deep.txt"} {
		if err := os.WriteFile(filepath.Join(local, path), []byte(path), 0600); err != nil {
			t.Fatal(err)
		}
	}
	transfer := &FileTransfer{LocalPath: local, RemotePath: remoteRoot}
	if err := transfer.Upload(connection); err == nil || !strings.Contains(err.Error(), "-r") {
		t.Fatalf("diretório aceito sem -r: %v", err)
	}
	transfer.Recursive = true
	if err := transfer.Upload(connection); err != nil {
		t.Fatal(err)
	}
	transfer.LocalPath, transfer.RemotePath, transfer.Recursive = filepath.Join(t.TempDir(), "download"), filepath.Join(remoteRoot, "tree"), false
	if err := transfer.Download(connection); err == nil || !strings.Contains(err.Error(), "-r") {
		t.Fatalf("download de diretório aceito sem -r: %v", err)
	}
	transfer.Recursive = true
	if err := transfer.Download(connection); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"top.txt", "nested/child.txt", "nested/deeper/deep.txt"} {
		assertTransferContent(t, filepath.Join(transfer.LocalPath, path), path)
	}
	transfer.LocalPath = filepath.Join(t.TempDir(), "inexistente")
	if err := transfer.Upload(connection); err == nil {
		t.Fatal("upload de arquivo inexistente aceito")
	}
	transfer.RemotePath = filepath.Join(remoteRoot, "inexistente")
	if err := transfer.Download(connection); err == nil {
		t.Fatal("download de arquivo inexistente aceito")
	}
}

func TestFileTransferUploadMultiple(t *testing.T) {
	first, firstKey, firstRoot := startTestSFTPServer(t, "selected-user")
	second, secondKey, secondRoot := startTestSFTPServer(t, "selected-user")
	cfg := &config.ConfigFile{Config: config.Config{User: []config.User{{Name: "selected-user"}}}}
	for i, address := range []string{first, second} {
		host, portText, err := net.SplitHostPort(address)
		if err != nil {
			t.Fatal(err)
		}
		port, err := strconv.Atoi(portText)
		if err != nil {
			t.Fatal(err)
		}
		cfg.Hosts = append(cfg.Hosts, config.Host{Name: fmt.Sprintf("server%d", i+1), Host: host, Port: port, Tags: []string{"web"}})
	}
	local := filepath.Join(t.TempDir(), "test.up")
	if err := os.WriteFile(local, []byte("conteúdo"), 0600); err != nil {
		t.Fatal(err)
	}
	transfer := &FileTransfer{LocalPath: local, RemotePath: "~", HostKeyOptions: HostKeyOptions{KnownHostsFile: trustedTestHostKeys(t, map[string]ssh.PublicKey{first: firstKey, second: secondKey})}}
	for _, hosts := range [][]string{{"@web"}, {"selected-user@" + first, second}} {
		var results []TransferResult
		output := captureCommandOutput(t, func() {
			results = transfer.UploadMultiple(cfg, hosts, &cfg.Config.User[0], nil, "secret", false)
			DisplayTransferResults(results, time.Second)
		})
		if !strings.Contains(output, "🚀 Enviando "+local+" para 2 host(s)") || strings.Count(output, "✅ User: selected-user Host:") != 2 || !strings.Contains(output, "📊 Resumo: 2 sucesso(s), 0 falha(s), 2 total") || strings.Contains(output, "\r") {
			t.Fatalf("envio em lote não usou o template consolidado: %s", output)
		}
		if transfer.QuietProgress {
			t.Fatal("envio em lote alterou a opção de progresso compartilhada")
		}
		if len(results) != 2 {
			t.Fatalf("resultados = %+v", results)
		}
		for _, result := range results {
			if result.User != "selected-user" || !result.Success || result.BytesSent != int64(len("conteúdo")) {
				t.Fatalf("transferência falhou: %+v", result)
			}
		}
		assertTransferContent(t, filepath.Join(firstRoot, "test.up"), "conteúdo")
		assertTransferContent(t, filepath.Join(secondRoot, "test.up"), "conteúdo")
	}
	for _, hosts := range [][]string{nil, {"@missing"}, {"host:porta-inválida"}} {
		results := transfer.UploadMultiple(cfg, hosts, &cfg.Config.User[0], nil, "secret", false)
		if len(results) != 1 || results[0].Success || results[0].Error == "" {
			t.Fatalf("entrada inválida aceita: %+v", results)
		}
	}
}

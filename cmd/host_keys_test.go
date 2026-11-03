package cmd

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/alexeiev/sshControl/config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func testHostKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func trustedTestHostKeys(t *testing.T, keys map[string]ssh.PublicKey) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "known_hosts")
	for hostname, key := range keys {
		if err := saveHostKey(filename, hostname, key); err != nil {
			t.Fatal(err)
		}
	}
	return filename
}

func TestHostKeyVerification(t *testing.T) {
	t.Parallel()
	key, changed := testHostKey(t), testHostKey(t)
	remote := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 2222}
	for _, hostname := range []string{"server.example:22", "server.example:2222", "[::1]:2222"} {
		t.Run(hostname, func(t *testing.T) {
			filename := trustedTestHostKeys(t, map[string]ssh.PublicKey{hostname: key})
			original, err := os.ReadFile(filename)
			if err != nil {
				t.Fatal(err)
			}
			check := newHostKeyCallback(filename, false, nil)
			if err := check(hostname, remote, key); err != nil {
				t.Fatalf("chave conhecida recusada: %v", err)
			}
			if err := check(hostname, remote, changed); err == nil || !strings.Contains(err.Error(), "--ignore-host-key") {
				t.Fatalf("chave divergente deveria ser recusada com orientação: %v", err)
			}
			if err := newHostKeyCallback(filename, true, nil)(hostname, remote, changed); err != nil {
				t.Fatalf("exceção explícita recusada: %v", err)
			}
			data, err := os.ReadFile(filename)
			if err != nil || string(data) != string(original) {
				t.Fatalf("exceção alterou known_hosts: %v", err)
			}
		})
	}
}

func TestUnknownHostKeyConfirmation(t *testing.T) {
	t.Parallel()
	key := testHostKey(t)
	hostname := "server.example:2222"
	remote := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 2222}
	for _, mode := range []string{"aceitar", "recusar", "sem terminal", "erro"} {
		t.Run(mode, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), ".ssh", "known_hosts")
			var confirm func(string, ssh.PublicKey) (bool, error)
			if mode != "sem terminal" {
				confirm = func(host string, presented ssh.PublicKey) (bool, error) {
					if host != hostname || ssh.FingerprintSHA256(presented) != ssh.FingerprintSHA256(key) {
						t.Fatal("confirmação recebeu host ou chave incorretos")
					}
					if mode == "erro" {
						return false, errors.New("leitura interrompida")
					}
					return mode == "aceitar", nil
				}
			}
			// A flag de divergência não deve aceitar um host desconhecido sozinha.
			err := newHostKeyCallback(filename, true, confirm)(hostname, remote, key)
			if mode != "aceitar" {
				if err == nil {
					t.Fatal("chave desconhecida aceita sem confirmação")
				}
				if _, err := os.Stat(filename); !os.IsNotExist(err) {
					t.Fatalf("arquivo criado sem confirmação: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := newHostKeyCallback(filename, false, nil)(hostname, remote, key); err != nil {
				t.Fatalf("chave confirmada não persistiu: %v", err)
			}
			info, err := os.Stat(filename)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatalf("permissão do known_hosts inválida: %v", err)
			}
		})
	}
}

func TestHashedKnownHostKey(t *testing.T) {
	t.Parallel()
	key := testHostKey(t)
	hostname := "server.example:2222"
	hashed := knownhosts.HashHostname(knownhosts.Normalize(hostname))
	filename := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(filename, []byte(knownhosts.Line([]string{hashed}, key)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := newHostKeyCallback(filename, false, nil)(hostname, &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 2222}, key); err != nil {
		t.Fatalf("entrada OpenSSH com hash recusada: %v", err)
	}
}

func TestHostKeyFileDefaultsIndependentlyOfSCCONFIG(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SCCONFIG", filepath.Join(t.TempDir(), "outro.yaml"))
	key := testHostKey(t)
	hostname := "server.example:22"
	filename := filepath.Join(home, ".ssh", "known_hosts")
	if err := saveHostKey(filename, hostname, key); err != nil {
		t.Fatal(err)
	}
	connection := &SSHConnection{InteractivePasswordAllowed: false}
	check, err := connection.hostKeyCallback()
	if err != nil {
		t.Fatal(err)
	}
	if err := check(hostname, &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 22}, key); err != nil {
		t.Fatalf("known_hosts padrão não encontrado: %v", err)
	}
}

func TestSavingNewHostPreservesExistingFileWithoutFinalNewline(t *testing.T) {
	t.Parallel()
	key := testHostKey(t)
	filename := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(filename, []byte(knownhosts.Line([]string{"old.example:22"}, key)), 0600); err != nil {
		t.Fatal(err)
	}
	check := newHostKeyCallback(filename, false, func(string, ssh.PublicKey) (bool, error) { return true, nil })
	remote := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 22}
	if err := check("new.example:22", remote, key); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"old.example:22", "new.example:22"} {
		if err := newHostKeyCallback(filename, false, nil)(host, remote, key); err != nil {
			t.Fatalf("chave de %s não preservada: %v", host, err)
		}
	}
}

func TestRevokedAndInvalidHostKeysAreNeverIgnored(t *testing.T) {
	t.Parallel()
	key := testHostKey(t)
	hostname := "server.example:22"
	remote := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 22}
	for _, data := range []string{"@revoked " + knownhosts.Line([]string{hostname}, key), "server.example ssh-ed25519 inválido"} {
		filename := filepath.Join(t.TempDir(), "known_hosts")
		if err := os.WriteFile(filename, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		check := newHostKeyCallback(filename, true, func(string, ssh.PublicKey) (bool, error) {
			t.Fatal("chave revogada ou arquivo inválido não deve oferecer confirmação")
			return true, nil
		})
		if err := check(hostname, remote, key); err == nil {
			t.Fatal("revogação ou arquivo inválido foi ignorado")
		}
	}
}

func TestConcurrentUnknownHostIsConfirmedOnce(t *testing.T) {
	t.Parallel()
	filename := filepath.Join(t.TempDir(), "known_hosts")
	key := testHostKey(t)
	var confirmations atomic.Int32
	check := newHostKeyCallback(filename, false, func(string, ssh.PublicKey) (bool, error) {
		confirmations.Add(1)
		return true, nil
	})
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if err := check("server.example:22", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 22}, key); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if confirmations.Load() != 1 {
		t.Fatalf("confirmações = %d; esperado 1", confirmations.Load())
	}
}

func TestHostKeyChecksDuringDirectAndJumpHandshakes(t *testing.T) {
	t.Parallel()
	target, targetKey := startTestSSHServer(t, "user", "secret")
	jump, jumpKey := startTestSSHServer(t, "user", "secret")
	host, portText, err := net.SplitHostPort(target)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	jumpHost, jumpPortText, err := net.SplitHostPort(jump)
	if err != nil {
		t.Fatal(err)
	}
	jumpPort, err := strconv.Atoi(jumpPortText)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"direct", "jump", "target changed", "jump changed", "ignore changed"} {
		t.Run(mode, func(t *testing.T) {
			keys := map[string]ssh.PublicKey{target: targetKey, jump: jumpKey}
			if mode == "target changed" || mode == "ignore changed" {
				keys[target] = testHostKey(t)
			}
			if mode == "jump changed" || mode == "ignore changed" {
				keys[jump] = testHostKey(t)
			}
			options := HostKeyOptions{KnownHostsFile: trustedTestHostKeys(t, keys), IgnoreMismatch: mode == "ignore changed"}
			var via *config.JumpHost
			if mode != "direct" {
				via = &config.JumpHost{Name: "jump", Host: jumpHost, Port: jumpPort, User: "user"}
			}
			connection := NewSSHConnection("user", host, port, nil, "secret", via, nil, "", false, "", 0, false, options)
			connection.InteractivePasswordAllowed = false
			cfg, err := connection.createSSHConfig()
			if err != nil {
				t.Fatal(err)
			}
			client, err := connection.dial(cfg)
			if mode == "target changed" || mode == "jump changed" {
				if err == nil {
					_ = client.Close()
					t.Fatal("handshake aceitou chave divergente")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := client.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

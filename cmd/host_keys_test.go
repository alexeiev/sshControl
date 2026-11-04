package cmd

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/alexeiev/sshControl/config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestHostKeyAlgorithmNegotiation(t *testing.T) {
	t.Parallel()
	_, edKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	edSigner, err := ssh.NewSignerFromKey(edKey)
	if err != nil {
		t.Fatal(err)
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ecSigner, err := ssh.NewSignerFromKey(ecKey)
	if err != nil {
		t.Fatal(err)
	}
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rsaSigner, err := ssh.NewSignerFromKey(rsaKey)
	if err != nil {
		t.Fatal(err)
	}
	legacyRSA, err := ssh.NewSignerWithAlgorithms(rsaSigner.(ssh.AlgorithmSigner), []string{ssh.KeyAlgoRSA})
	if err != nil {
		t.Fatal(err)
	}
	certificate := &ssh.Certificate{
		Key:             edSigner.PublicKey(),
		CertType:        ssh.HostCert,
		ValidPrincipals: []string{"127.0.0.1"},
		ValidBefore:     ssh.CertTimeInfinity,
	}
	if err := certificate.SignCert(rand.Reader, rsaSigner); err != nil {
		t.Fatal(err)
	}
	certSigner, err := ssh.NewCertSigner(certificate, edSigner)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name          string
		known         ssh.PublicKey
		marker        string
		hashed        bool
		serverSigners []ssh.Signer
		jumpECDSA     bool
		want          string
		wantReject    bool
	}{
		{name: "novo servidor prefere Ed25519", want: ssh.KeyAlgoED25519},
		{name: "entrada Ed25519 do OpenSSH", known: edSigner.PublicKey(), want: ssh.KeyAlgoED25519},
		{name: "entrada Ed25519 com hash", known: edSigner.PublicKey(), hashed: true, want: ssh.KeyAlgoED25519},
		{name: "entrada ECDSA existente do sc", known: ecSigner.PublicKey(), want: ssh.KeyAlgoECDSA521},
		{name: "entrada RSA existente usa SHA-2", known: rsaSigner.PublicKey(), want: ssh.KeyAlgoRSA},
		{name: "servidor somente ECDSA", serverSigners: []ssh.Signer{ecSigner}, want: ssh.KeyAlgoECDSA521},
		{name: "servidor somente RSA", serverSigners: []ssh.Signer{rsaSigner}, want: ssh.KeyAlgoRSA},
		{name: "compatibilidade RSA legado preservada", serverSigners: []ssh.Signer{legacyRSA}, want: ssh.KeyAlgoRSA},
		{name: "destino Ed25519 e jump ECDSA", known: edSigner.PublicKey(), jumpECDSA: true, want: ssh.KeyAlgoED25519},
		{name: "certificado mantém prioridade com CA", known: rsaSigner.PublicKey(), marker: "@cert-authority ", serverSigners: []ssh.Signer{ecSigner, edSigner, certSigner}, want: ssh.CertAlgoED25519v01},
		{name: "Ed25519 alterada continua bloqueada", known: testHostKey(t), wantReject: true},
	} {
		for _, viaJump := range []bool{false, true} {
			if tc.jumpECDSA && !viaJump {
				continue
			}
			t.Run(tc.name+"/jump="+strconv.FormatBool(viaJump), func(t *testing.T) {
				signers := tc.serverSigners
				if len(signers) == 0 {
					signers = []ssh.Signer{ecSigner, edSigner, rsaSigner}
				}
				target, _ := startTestSSHServer(t, "user", "secret", signers...)
				addresses := []string{target}
				var jumpHost *config.JumpHost
				if viaJump {
					jump, _ := startTestSSHServer(t, "user", "secret", signers...)
					addresses = append(addresses, jump)
					host, portText, err := net.SplitHostPort(jump)
					if err != nil {
						t.Fatal(err)
					}
					port, err := strconv.Atoi(portText)
					if err != nil {
						t.Fatal(err)
					}
					jumpHost = &config.JumpHost{Name: "jump", Host: host, Port: port, User: "user"}
				}
				filename := filepath.Join(t.TempDir(), "known_hosts")
				var original []byte
				if tc.known != nil {
					for _, address := range addresses {
						known := tc.known
						if tc.jumpECDSA && address != target {
							known = ecSigner.PublicKey()
						}
						if tc.hashed {
							address = knownhosts.HashHostname(knownhosts.Normalize(address))
						}
						original = append(original, tc.marker+knownhosts.Line([]string{address}, known)+"\n"...)
					}
					if err := os.WriteFile(filename, original, 0600); err != nil {
						t.Fatal(err)
					}
				}
				host, portText, err := net.SplitHostPort(target)
				if err != nil {
					t.Fatal(err)
				}
				port, err := strconv.Atoi(portText)
				if err != nil {
					t.Fatal(err)
				}
				connection := NewSSHConnection("user", host, port, nil, "secret", jumpHost, nil, "", false, "", 0, false, HostKeyOptions{KnownHostsFile: filename})
				connection.InteractivePasswordAllowed = false
				cfg, err := connection.createSSHConfig()
				if err != nil {
					t.Fatal(err)
				}
				check := cfg.HostKeyCallback
				cfg.HostKeyCallback = func(hostname string, remote net.Addr, key ssh.PublicKey) error {
					if !tc.wantReject && key.Type() != tc.want {
						t.Errorf("algoritmo negociado = %s; esperado %s", key.Type(), tc.want)
					}
					return check(hostname, remote, key)
				}
				client, err := connection.dial(cfg)
				if tc.wantReject {
					if err == nil {
						_ = client.Close()
						t.Fatal("chave alterada aceita")
					}
					return
				}
				if err != nil {
					t.Fatalf("negociação recusou chave legítima: %v", err)
				}
				if err := client.Close(); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(filename)
				if err != nil {
					t.Fatal(err)
				}
				if tc.known != nil && string(data) != string(original) {
					t.Fatal("negociação alterou entrada existente")
				}
				if tc.known == nil && strings.Count(string(data), " "+tc.want+" ") != len(addresses) {
					t.Fatalf("algoritmo cadastrado inesperado: %s", data)
				}
			})
		}
	}
}

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
			check := newHostKeyCallback(filename, false)
			if err := check(hostname, remote, key); err != nil {
				t.Fatalf("chave conhecida recusada: %v", err)
			}
			if err := check(hostname, remote, changed); err == nil || !strings.Contains(err.Error(), "--ignore-host-key") {
				t.Fatalf("chave divergente deveria ser recusada com orientação: %v", err)
			}
			if err := newHostKeyCallback(filename, true)(hostname, remote, changed); err != nil {
				t.Fatalf("exceção explícita recusada: %v", err)
			}
			data, err := os.ReadFile(filename)
			if err != nil || string(data) != string(original) {
				t.Fatalf("exceção alterou known_hosts: %v", err)
			}
		})
	}
}

func TestUnknownHostKeyIsAcceptedAndSaved(t *testing.T) {
	t.Parallel()
	key := testHostKey(t)
	hostname := "server.example:2222"
	remote := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 2222}
	for _, ignoreMismatch := range []bool{false, true} {
		t.Run(strconv.FormatBool(ignoreMismatch), func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), ".ssh", "known_hosts")
			if err := newHostKeyCallback(filename, ignoreMismatch)(hostname, remote, key); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filename)
			if err != nil || !strings.Contains(string(data), knownhosts.Line([]string{hostname}, key)) {
				t.Fatalf("chave nova não persistiu: %v", err)
			}
			if err := newHostKeyCallback(filename, false)(hostname, remote, key); err != nil {
				t.Fatalf("chave cadastrada recusada: %v", err)
			}
			if err := newHostKeyCallback(filename, false)(hostname, remote, testHostKey(t)); err == nil {
				t.Fatal("chave diferente aceita após o cadastro automático")
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
	if err := newHostKeyCallback(filename, false)(hostname, &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 2222}, key); err != nil {
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
	check := newHostKeyCallback(filename, false)
	remote := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 22}
	if err := check("new.example:22", remote, key); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"old.example:22", "new.example:22"} {
		if err := newHostKeyCallback(filename, false)(host, remote, key); err != nil {
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
		check := newHostKeyCallback(filename, true)
		if err := check(hostname, remote, key); err == nil {
			t.Fatal("revogação ou arquivo inválido foi ignorado")
		}
	}
}

func TestConcurrentUnknownHostIsSavedOnce(t *testing.T) {
	t.Parallel()
	filename := filepath.Join(t.TempDir(), "known_hosts")
	key := testHostKey(t)
	check := newHostKeyCallback(filename, false)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if err := check("server.example:22", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 22}, key); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(data), knownhosts.Line([]string{"server.example:22"}, key)); count != 1 {
		t.Fatalf("entradas cadastradas = %d; esperado 1", count)
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
	for _, mode := range []string{"direct", "jump", "new hosts", "target changed", "jump changed", "ignore changed"} {
		t.Run(mode, func(t *testing.T) {
			keys := map[string]ssh.PublicKey{target: targetKey, jump: jumpKey}
			if mode == "target changed" || mode == "ignore changed" {
				keys[target] = testHostKey(t)
			}
			if mode == "jump changed" || mode == "ignore changed" {
				keys[jump] = testHostKey(t)
			}
			options := HostKeyOptions{KnownHostsFile: trustedTestHostKeys(t, keys), IgnoreMismatch: mode == "ignore changed"}
			if mode == "new hosts" {
				options.KnownHostsFile = filepath.Join(t.TempDir(), "known_hosts")
			}
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
			if mode == "new hosts" {
				data, err := os.ReadFile(options.KnownHostsFile)
				if err != nil {
					t.Fatal(err)
				}
				for address, key := range keys {
					if !strings.Contains(string(data), knownhosts.Line([]string{address}, key)) {
						t.Fatalf("chave de %s não cadastrada automaticamente", address)
					}
				}
			}
			if err := client.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

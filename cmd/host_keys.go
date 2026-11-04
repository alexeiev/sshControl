package cmd

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// HostKeyOptions controla a verificação de identidade dos servidores SSH.
type HostKeyOptions struct {
	IgnoreMismatch bool
	KnownHostsFile string // Vazio usa ~/.ssh/known_hosts, independente de SCCONFIG.
}

// Serializa atualizações feitas pelas conexões deste processo.
var hostKeyMu sync.Mutex

func (s *SSHConnection) hostKeyCallback() (ssh.HostKeyCallback, error) {
	filename, err := s.hostKeyFile()
	if err != nil {
		return nil, err
	}
	return newHostKeyCallback(filename, s.HostKeyOptions.IgnoreMismatch), nil
}

func (s *SSHConnection) hostKeyFile() (string, error) {
	filename := s.HostKeyOptions.KnownHostsFile
	if filename == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("erro ao obter diretório home para known_hosts: %w", err)
		}
		filename = filepath.Join(home, ".ssh", "known_hosts")
	}
	return filename, nil
}

// A sonda nunca participa do handshake. Seu conteúdo vazio não corresponde a
// nenhuma chave válida e permite consultar KeyError.Want com o parser oficial,
// preservando o suporte a hashes, padrões de hosts e portas no known_hosts.
type hostKeyAlgorithmProbe struct{}

func (hostKeyAlgorithmProbe) Type() string    { return "" }
func (hostKeyAlgorithmProbe) Marshal() []byte { return nil }
func (hostKeyAlgorithmProbe) Verify([]byte, *ssh.Signature) error {
	return errors.New("sonda de algoritmos não verifica assinaturas")
}

func (s *SSHConnection) hostKeyAlgorithms(address string) ([]string, error) {
	filename, err := s.hostKeyFile()
	if err != nil {
		return nil, err
	}
	hostKeyMu.Lock()
	defer hostKeyMu.Unlock()

	knownTypes := make(map[string]bool)
	check, err := knownhosts.New(filename)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("erro ao ler %s: %w", filename, err)
	}
	if err == nil {
		// O endereço consultado tem precedência sobre o endereço remoto fictício.
		err = check(address, &net.TCPAddr{}, hostKeyAlgorithmProbe{})
		var keyErr *knownhosts.KeyError
		if !errors.As(err, &keyErr) {
			return nil, fmt.Errorf("erro ao consultar algoritmos de %s: %w", address, err)
		}
		for _, known := range keyErr.Want {
			knownTypes[known.Key.Type()] = true
		}
	}

	// Preferência do OpenSSH para os algoritmos modernos da biblioteca.
	// Certificados continuam prioritários para preservar a autenticação por CA.
	algorithms := []string{
		ssh.CertAlgoED25519v01,
		ssh.CertAlgoECDSA256v01, ssh.CertAlgoECDSA384v01, ssh.CertAlgoECDSA521v01,
		ssh.CertAlgoRSASHA512v01, ssh.CertAlgoRSASHA256v01,
	}
	plain := []string{
		ssh.KeyAlgoED25519,
		ssh.KeyAlgoECDSA256, ssh.KeyAlgoECDSA384, ssh.KeyAlgoECDSA521,
		ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256,
	}
	// Preserva as alternativas legadas do padrão anterior da biblioteca.
	for _, algorithm := range ssh.InsecureAlgorithms().HostKeys {
		if strings.Contains(algorithm, "-cert-") {
			algorithms = append(algorithms, algorithm)
		} else {
			plain = append(plain, algorithm)
		}
	}
	var remaining []string
	for _, algorithm := range plain {
		keyType := algorithm
		if algorithm == ssh.KeyAlgoRSASHA512 || algorithm == ssh.KeyAlgoRSASHA256 {
			// As assinaturas RSA/SHA-2 usam entradas ssh-rsa no known_hosts.
			keyType = ssh.KeyAlgoRSA
		}
		if knownTypes[keyType] {
			algorithms = append(algorithms, algorithm)
		} else {
			remaining = append(remaining, algorithm)
		}
	}
	return append(algorithms, remaining...), nil
}

func newHostKeyCallback(filename string, ignoreMismatch bool) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		hostKeyMu.Lock()
		defer hostKeyMu.Unlock()

		// Recarrega a base para observar chaves aceitas por outras conexões.
		check, err := knownhosts.New(filename)
		if os.IsNotExist(err) {
			check, err = knownhosts.New()
		}
		if err != nil {
			return fmt.Errorf("erro ao ler %s: %w", filename, err)
		}
		err = check(hostname, remote, key)
		if err == nil {
			return nil
		}
		var keyErr *knownhosts.KeyError
		if !errors.As(err, &keyErr) {
			// Revogação e certificados inválidos nunca são ignorados.
			return fmt.Errorf("falha ao validar chave de %s: %w", hostname, err)
		}
		fingerprint := ssh.FingerprintSHA256(key)
		if len(keyErr.Want) > 0 {
			if !ignoreMismatch {
				return fmt.Errorf("chave SSH diferente para %s: recebida %s; confira %s. Após verificar a identidade do servidor, use --ignore-host-key nesta execução ou remova a entrada antiga do known_hosts: %w", hostname, fingerprint, filename, err)
			}
			fmt.Fprintf(os.Stderr, "⚠️  Chave SSH diferente para %s (%s) aceita por --ignore-host-key; %s não será alterado.\n", hostname, fingerprint, filename)
			return nil
		}
		if err := saveHostKey(filename, hostname, key); err != nil {
			return fmt.Errorf("erro ao salvar chave de %s: %w", hostname, err)
		}
		fmt.Fprintf(os.Stderr, "✓ Chave SSH de %s (%s) adicionada a %s.\n", hostname, fingerprint, filename)
		return nil
	}
}

func saveHostKey(filename, hostname string, key ssh.PublicKey) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	// A quebra inicial preserva arquivos existentes sem uma quebra final.
	_, writeErr := file.WriteString("\n" + knownhosts.Line([]string{hostname}, key) + "\n")
	return errors.Join(writeErr, file.Close())
}

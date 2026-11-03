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
	"golang.org/x/term"
)

// HostKeyOptions controla a verificação de identidade dos servidores SSH.
type HostKeyOptions struct {
	IgnoreMismatch bool
	KnownHostsFile string // Vazio usa ~/.ssh/known_hosts, independente de SCCONFIG.
}

// Serializa confirmações e atualizações feitas pelas conexões deste processo.
var hostKeyMu sync.Mutex

func (s *SSHConnection) hostKeyCallback() (ssh.HostKeyCallback, error) {
	filename := s.HostKeyOptions.KnownHostsFile
	if filename == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("erro ao obter diretório home para known_hosts: %w", err)
		}
		filename = filepath.Join(home, ".ssh", "known_hosts")
	}
	var confirm func(string, ssh.PublicKey) (bool, error)
	if s.InteractivePasswordAllowed && term.IsTerminal(int(os.Stdin.Fd())) {
		confirm = confirmUnknownHostKey
	}
	return newHostKeyCallback(filename, s.HostKeyOptions.IgnoreMismatch, confirm), nil
}

func newHostKeyCallback(filename string, ignoreMismatch bool, confirm func(string, ssh.PublicKey) (bool, error)) ssh.HostKeyCallback {
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
				return fmt.Errorf("chave SSH diferente para %s: recebida %s; confira %s. Para ignorar somente nesta execução, use --ignore-host-key: %w", hostname, fingerprint, filename, err)
			}
			fmt.Fprintf(os.Stderr, "⚠️  Chave SSH diferente para %s (%s) aceita por --ignore-host-key; %s não será alterado.\n", hostname, fingerprint, filename)
			return nil
		}
		if confirm == nil {
			return fmt.Errorf("chave SSH desconhecida para %s (%s); confirme primeiro em uma execução interativa (para tunnel, use -v) ou cadastre a chave verificada em %s: %w", hostname, fingerprint, filename, err)
		}
		accepted, err := confirm(hostname, key)
		if err != nil {
			return fmt.Errorf("erro ao confirmar chave de %s: %w", hostname, err)
		}
		if !accepted {
			return fmt.Errorf("chave SSH de %s não aceita", hostname)
		}
		if err := saveHostKey(filename, hostname, key); err != nil {
			return fmt.Errorf("erro ao salvar chave de %s: %w", hostname, err)
		}
		return nil
	}
}

func confirmUnknownHostKey(hostname string, key ssh.PublicKey) (bool, error) {
	fmt.Fprintf(os.Stderr, "Chave SSH desconhecida para %s.\nTipo: %s\nFingerprint: %s\nConfiar nesta chave e salvar em known_hosts? [s/N]: ", hostname, key.Type(), ssh.FingerprintSHA256(key))
	var answer string
	if _, err := fmt.Fscanln(os.Stdin, &answer); err != nil {
		return false, err
	}
	return strings.EqualFold(answer, "s") || strings.EqualFold(answer, "sim") || strings.EqualFold(answer, "yes"), nil
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

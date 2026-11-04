package cmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/alexeiev/sshControl/config"
	"golang.org/x/crypto/ssh"
)

func TestCommandResultsPreserveReferenceTemplate(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	writeCommandResults(&output, []HostResult{
		{User: "ubuntu", Host: "100.64.2.11", Success: true, Output: "Minha maquina remota é: srv-ansible-cli01\n"},
		{User: "devops", Host: "100.64.2.12", Success: true, Output: "Minha maquina remota é: srv-ansible-cli02"},
	}, 600*time.Millisecond)
	want := `━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
✅ User: ubuntu Host: 100.64.2.11 (Exit Code: 0)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Minha maquina remota é: srv-ansible-cli01

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
✅ User: devops Host: 100.64.2.12 (Exit Code: 0)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Minha maquina remota é: srv-ansible-cli02

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
📊 Resumo: 2 sucesso(s), 0 falha(s), 2 total | ⏱️  Tempo: 0.60s
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
`
	if output.String() != want {
		t.Fatalf("template diferente da referência:\n%s", output.String())
	}
}

func TestTransferResultsUseReferenceTemplate(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	writeTransferResults(&output, []TransferResult{
		{User: "ubuntu", Host: "100.64.2.11", Success: true, FilePath: "/tmp/test.up", BytesSent: 46, Duration: 800 * time.Millisecond},
		{User: "devops", Host: "100.64.2.12", Error: "acesso negado", Duration: 300 * time.Millisecond},
	}, 900*time.Millisecond)
	for _, want := range []string{
		outputSeparator + "\n✅ User: ubuntu Host: 100.64.2.11 (Tempo: 0.8s)\n" + outputSeparator,
		"Caminho: /tmp/test.up (46B)\n\n",
		outputSeparator + "\n❌ User: devops Host: 100.64.2.12 (Tempo: 0.3s)\n" + outputSeparator,
		"Erro: acesso negado\n\n",
		"📊 Resumo: 1 sucesso(s), 1 falha(s), 2 total | ⏱️  Tempo: 0.90s",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("ausente %q:\n%s", want, output.String())
		}
	}
	if strings.Contains(output.String(), "Exit Code") || strings.Contains(output.String(), "----") {
		t.Fatalf("metadados de comando ou template antigo em transferência: %s", output.String())
	}
}

func TestOperationResultsFailuresAndEmptyResults(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	writeCommandResults(&output, []HostResult{{Host: "server", ExitCode: 7, Output: "saída parcial", Error: "comando falhou"}}, time.Second)
	for _, want := range []string{"❌ Host: server (Exit Code: 7)", "saída parcial\nErro: comando falhou\n", "0 sucesso(s), 1 falha(s), 1 total"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("ausente %q: %s", want, output.String())
		}
	}
	output.Reset()
	writeOperationResults(&output, nil, 0)
	if !strings.Contains(output.String(), "0 sucesso(s), 0 falha(s), 0 total") {
		t.Fatalf("resumo vazio incorreto: %s", output.String())
	}
}

func TestOperationHeaderIncludesJumpDetails(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	writeOperationHeader(&output, "Enviando /tmp/test.up para 2 host(s)", &config.JumpHost{Name: "backup-jump", User: "ubuntu", Host: "192.168.1.203", Port: 22})
	want := "\n🚀 Enviando /tmp/test.up para 2 host(s)\n   via Jump Host: backup-jump (ubuntu@192.168.1.203:22)\n\n"
	if output.String() != want {
		t.Fatalf("cabeçalho = %q; esperado %q", output.String(), want)
	}
	output.Reset()
	writeOperationHeader(&output, "Baixando arquivo", nil)
	if strings.Contains(output.String(), "Jump Host") {
		t.Fatal("jump host inexistente exibido")
	}
}

// Não usa t.Parallel: a captura é limitada aos testes da saída pública da CLI.
func captureCommandOutput(t *testing.T, run func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = original; _ = writer.Close(); _ = reader.Close() })
	content := make(chan string, 1)
	go func() { data, _ := io.ReadAll(reader); content <- string(data) }()
	run()
	_ = writer.Close()
	os.Stdout = original
	return <-content
}

func TestQuietTransferCopiesDataWithoutProgressLines(t *testing.T) {
	var destination bytes.Buffer
	transfer := &FileTransfer{QuietProgress: true}
	output := captureCommandOutput(t, func() {
		if err := transfer.copyWithProgress(&destination, strings.NewReader("conteúdo"), "test.up", "server", int64(len("conteúdo"))); err != nil {
			t.Error(err)
		}
	})
	if output != "" || destination.String() != "conteúdo" {
		t.Fatalf("saída=%q; dados=%q", output, destination.String())
	}
	wantErr := errors.New("falha de leitura")
	if err := transfer.copyWithProgress(io.Discard, iotest.ErrReader(wantErr), "test.up", "server", 1); !errors.Is(err, wantErr) {
		t.Fatalf("erro de cópia não preservado: %v", err)
	}
}

func serveTestCommandSession(channel ssh.Channel, requests <-chan *ssh.Request) {
	defer func() { _ = channel.Close() }()
	for request := range requests {
		var payload struct{ Command string }
		if request.Type != "exec" || ssh.Unmarshal(request.Payload, &payload) != nil {
			_ = request.Reply(false, nil)
			continue
		}
		_ = request.Reply(true, nil)
		output := "saída do servidor\n"
		if payload.Command == "test-no-newline" {
			output = strings.TrimSuffix(output, "\n")
		}
		_, _ = io.WriteString(channel, output)
		status := uint32(0)
		if payload.Command == "test-failure" {
			status = 7
		}
		_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
		return
	}
}

func TestSingleCommandUsesSharedResultTemplate(t *testing.T) {
	for _, tc := range []struct {
		command, heading, summary string
		wantError                 bool
	}{
		{"test-success", "✅ User: selected-user Host:", "1 sucesso(s), 0 falha(s), 1 total", false},
		{"test-no-newline", "✅ User: selected-user Host:", "1 sucesso(s), 0 falha(s), 1 total", false},
		{"test-failure", "❌ User: selected-user Host:", "0 sucesso(s), 1 falha(s), 1 total", true},
	} {
		t.Run(tc.command, func(t *testing.T) {
			address, key := startTestSSHServer(t, "selected-user", "secret")
			connection := transferTestConnection(t, address, trustedTestHostKeys(t, map[string]ssh.PublicKey{address: key}), nil)
			connection.Command = tc.command
			var commandErr error
			output := captureCommandOutput(t, func() { commandErr = connection.ExecuteCommand() })
			if (commandErr != nil) != tc.wantError {
				t.Fatalf("erro inesperado: %v", commandErr)
			}
			for _, want := range []string{"🚀 Executando comando em 1 host(s): " + tc.command, "saída do servidor\n", tc.heading, tc.summary} {
				if !strings.Contains(output, want) {
					t.Fatalf("ausente %q: %s", want, output)
				}
			}
			if tc.wantError && !strings.Contains(output, "Exit Code: 7") {
				t.Fatalf("exit code perdido: %s", output)
			}
			if strings.Index(output, "saída do servidor") > strings.Index(output, tc.heading) {
				t.Fatal("saída em tempo real mudou de posição")
			}
		})
	}
}

func TestResultsReportEffectiveRemoteUser(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, user, password string
		explicit             bool
		wantSuccess          bool
	}{
		{"usuário selecionado", "selected-user", "secret", false, true},
		{"usuário explícito no destino", "explicit-user", "secret", true, true},
		{"falha de autenticação", "explicit-user", "senha-incorreta", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.ConfigFile{}
			selected := &config.User{Name: "selected-user"}
			address, key := startTestSSHServer(t, tc.user, "secret")
			host := address
			if tc.explicit {
				host = tc.user + "@" + address
			}
			options := HostKeyOptions{KnownHostsFile: trustedTestHostKeys(t, map[string]ssh.PublicKey{address: key})}
			command := executeOnHost(cfg, host, selected, nil, tc.password, "test-success", false, "", 0, false, false, options)
			if command.User != tc.user || command.Success != tc.wantSuccess {
				t.Fatalf("usuário do comando não corresponde à conexão: %+v", command)
			}

			address, key, _ = startTestSFTPServer(t, tc.user)
			host = address
			if tc.explicit {
				host = tc.user + "@" + address
			}
			local := t.TempDir() + "/test.up"
			if err := os.WriteFile(local, []byte("teste"), 0600); err != nil {
				t.Fatal(err)
			}
			transfer := &FileTransfer{LocalPath: local, RemotePath: "~", HostKeyOptions: HostKeyOptions{KnownHostsFile: trustedTestHostKeys(t, map[string]ssh.PublicKey{address: key})}}
			result := transfer.uploadToHost(cfg, host, selected, nil, tc.password)
			if result.User != tc.user || result.Success != tc.wantSuccess {
				t.Fatalf("usuário da transferência não corresponde à conexão: %+v", result)
			}
		})
	}
}

package cmd

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/alexeiev/sshControl/config"
)

const outputSeparator = "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

// operationResult contém somente os dados de apresentação de cada host.
type operationResult struct {
	User, Host, Detail, Output, Error string
	Success                           bool
}

// PrintOperationHeader apresenta a operação e a conexão via jump host.
func PrintOperationHeader(description string, jumpHost *config.JumpHost) {
	writeOperationHeader(os.Stdout, description, jumpHost)
}

func (s *SSHConnection) printConnectionHeader() {
	description := *s
	description.JumpHost = nil
	PrintOperationHeader("Conectando: "+description.formatConnectionString(), s.JumpHost)
}

func writeOperationHeader(w io.Writer, description string, jumpHost *config.JumpHost) {
	_, _ = fmt.Fprintf(w, "\n🚀 %s\n", description)
	if jumpHost != nil {
		_, _ = fmt.Fprintf(w, "   via Jump Host: %s (%s@%s:%d)\n", jumpHost.Name, jumpHost.User, jumpHost.Host, jumpHost.Port)
	}
	_, _ = fmt.Fprintln(w)
}

func writeOperationResults(w io.Writer, results []operationResult, duration time.Duration) {
	successCount := 0
	for _, result := range results {
		icon := "❌"
		if result.Success {
			icon = "✅"
			successCount++
		}
		_, _ = fmt.Fprintln(w, outputSeparator)
		_, _ = fmt.Fprintf(w, "%s ", icon)
		if result.User != "" {
			_, _ = fmt.Fprintf(w, "User: %s ", result.User)
		}
		_, _ = fmt.Fprintf(w, "Host: %s", result.Host)
		if result.Detail != "" {
			_, _ = fmt.Fprintf(w, " (%s)", result.Detail)
		}
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, outputSeparator)
		if result.Output != "" {
			_, _ = fmt.Fprint(w, result.Output)
			if result.Output[len(result.Output)-1] != '\n' {
				_, _ = fmt.Fprintln(w)
			}
		}
		if result.Error != "" {
			_, _ = fmt.Fprintf(w, "Erro: %s\n", result.Error)
		}
		_, _ = fmt.Fprintln(w)
	}
	_, _ = fmt.Fprintln(w, outputSeparator)
	_, _ = fmt.Fprintf(w, "📊 Resumo: %d sucesso(s), %d falha(s), %d total | ⏱️  Tempo: %.2fs\n", successCount, len(results)-successCount, len(results), duration.Seconds())
	_, _ = fmt.Fprintln(w, outputSeparator)
}

func writeCommandResults(w io.Writer, results []HostResult, duration time.Duration) {
	formatted := make([]operationResult, 0, len(results))
	for _, result := range results {
		formatted = append(formatted, operationResult{
			User: result.User, Host: result.Host, Success: result.Success,
			Detail: fmt.Sprintf("Exit Code: %d", result.ExitCode),
			Output: result.Output, Error: result.Error,
		})
	}
	writeOperationResults(w, formatted, duration)
}

func writeTransferResults(w io.Writer, results []TransferResult, duration time.Duration) {
	formatted := make([]operationResult, 0, len(results))
	for _, result := range results {
		output := ""
		if result.FilePath != "" {
			output = fmt.Sprintf("Caminho: %s", result.FilePath)
			if result.Success && result.BytesSent > 0 {
				output += fmt.Sprintf(" (%s)", formatBytes(result.BytesSent))
			}
		}
		formatted = append(formatted, operationResult{
			User: result.User, Host: result.Host, Success: result.Success,
			Detail: fmt.Sprintf("Tempo: %.1fs", result.Duration.Seconds()),
			Output: output, Error: result.Error,
		})
	}
	writeOperationResults(w, formatted, duration)
}

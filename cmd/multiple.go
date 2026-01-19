package cmd

import (
	"bytes"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ceiev/sshControl/config"
	"golang.org/x/crypto/ssh"
)

// HostResult armazena o resultado da execução em um host
type HostResult struct {
	Host     string
	Success  bool
	Output   string
	Error    string
	ExitCode int
}

// ConnectMultiple executa um comando em múltiplos hosts em paralelo
func ConnectMultiple(cfg *config.ConfigFile, hostArgs []string, selectedUser *config.User, useJumpHost bool, command string) {
	// Determina o usuário efetivo
	effectiveUser := cfg.GetEffectiveUser(selectedUser)
	if effectiveUser == nil {
		fmt.Fprintf(os.Stderr, "Erro: Nenhum usuário configurado\n")
		os.Exit(1)
	}

	// Prepara o Jump Host
	jumpHost := ""
	if useJumpHost {
		jumpHost = cfg.Config.JumpHosts
	}

	fmt.Println()
	fmt.Printf("🚀 Executando comando em %d host(s): %s\n", len(hostArgs), command)
	fmt.Println()

	// Captura o tempo de início
	startTime := time.Now()

	// Canal para coletar resultados
	results := make(chan HostResult, len(hostArgs))
	var wg sync.WaitGroup

	// Executa comando em cada host em paralelo
	for _, hostArg := range hostArgs {
		wg.Add(1)
		go func(hostArg string) {
			defer wg.Done()
			result := executeOnHost(cfg, hostArg, effectiveUser, useJumpHost, jumpHost, command)
			results <- result
		}(hostArg)
	}

	// Aguarda todas as goroutines terminarem
	go func() {
		wg.Wait()
		close(results)
	}()

	// Coleta e exibe resultados
	var allResults []HostResult
	for result := range results {
		allResults = append(allResults, result)
	}

	// Calcula o tempo total de execução
	duration := time.Since(startTime)

	// Exibe resultados organizados
	displayResults(allResults, duration)
}

// executeOnHost executa o comando em um único host e retorna o resultado
func executeOnHost(cfg *config.ConfigFile, hostArg string, effectiveUser *config.User, useJumpHost bool, jumpHost string, command string) HostResult {
	var hostname string
	var port int
	var sshKey string

	username := effectiveUser.Name
	if len(effectiveUser.SSHKeys) > 0 {
		sshKey = config.ExpandHomePath(effectiveUser.SSHKeys[0])
	}

	// Primeiro tenta encontrar no config.yaml
	if host := cfg.FindHost(hostArg); host != nil {
		hostname = host.Host
		port = host.Port
	} else {
		// Se não encontrar, tenta parsear como conexão direta
		host, err := parseDirectConnection(hostArg, effectiveUser)
		if err != nil {
			return HostResult{
				Host:    hostArg,
				Success: false,
				Error:   fmt.Sprintf("Formato inválido: %v", err),
			}
		}

		// Se a string incluir um usuário explícito (user@host), usa ele
		if host.parsedUser != "" && host.parsedUser != effectiveUser.Name {
			username = host.parsedUser
			// Tenta obter a chave SSH desse usuário específico
			if userFromConfig := cfg.FindUser(username); userFromConfig != nil {
				if len(userFromConfig.SSHKeys) > 0 {
					sshKey = config.ExpandHomePath(userFromConfig.SSHKeys[0])
				}
			} else {
				sshKey = ""
			}
		}

		hostname = host.hostname
		port = host.port
	}

	// Cria a conexão SSH
	sshConn := NewSSHConnection(
		username,
		hostname,
		port,
		sshKey,
		useJumpHost,
		jumpHost,
		command,
	)

	// Executa o comando e captura a saída
	output, exitCode, err := sshConn.ExecuteCommandWithOutput()
	if err != nil {
		return HostResult{
			Host:     hostArg,
			Success:  false,
			Output:   output,
			Error:    err.Error(),
			ExitCode: exitCode,
		}
	}

	return HostResult{
		Host:     hostArg,
		Success:  true,
		Output:   output,
		ExitCode: exitCode,
	}
}

// displayResults exibe os resultados de forma organizada
func displayResults(results []HostResult, duration time.Duration) {
	successCount := 0
	failureCount := 0

	for _, result := range results {
		if result.Success {
			successCount++
		} else {
			failureCount++
		}

		// Cabeçalho do host
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		if result.Success {
			fmt.Printf("✅ Host: %s (Exit Code: %d)\n", result.Host, result.ExitCode)
		} else {
			fmt.Printf("❌ Host: %s (Exit Code: %d)\n", result.Host, result.ExitCode)
		}
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Exibe a saída
		if result.Output != "" {
			fmt.Print(result.Output)
			// Garante que há uma nova linha no final se não houver
			if result.Output[len(result.Output)-1] != '\n' {
				fmt.Println()
			}
		}

		// Exibe erro se houver
		if result.Error != "" {
			fmt.Printf("Erro: %s\n", result.Error)
		}

		fmt.Println()
	}

	// Resumo final
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("📊 Resumo: %d sucesso(s), %d falha(s), %d total | ⏱️  Tempo: %.2fs\n", successCount, failureCount, len(results), duration.Seconds())
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

// ExecuteCommandWithOutput executa um comando remoto e retorna a saída
func (s *SSHConnection) ExecuteCommandWithOutput() (output string, exitCode int, err error) {
	// Cria a configuração SSH
	config, err := s.createSSHConfig()
	if err != nil {
		return "", -1, fmt.Errorf("erro ao criar configuração SSH: %w", err)
	}

	// Conecta ao host (via Jump Host se necessário)
	client, err := s.dial(config)
	if err != nil {
		return "", -1, fmt.Errorf("erro ao conectar: %w", err)
	}
	defer client.Close()

	// Cria uma sessão SSH
	session, err := client.NewSession()
	if err != nil {
		return "", -1, fmt.Errorf("erro ao criar sessão: %w", err)
	}
	defer session.Close()

	// Buffers para capturar stdout e stderr
	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	// Executa o comando
	err = session.Run(s.Command)

	// Combina stdout e stderr
	combinedOutput := stdout.String()
	if stderr.Len() > 0 {
		combinedOutput += stderr.String()
	}

	// Captura o exit code
	exitCode = 0
	if err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			exitCode = exitErr.ExitStatus()
			// Se temos um exit code, não é um erro de conexão
			return combinedOutput, exitCode, nil
		}
		return combinedOutput, -1, fmt.Errorf("erro ao executar comando: %w", err)
	}

	return combinedOutput, exitCode, nil
}

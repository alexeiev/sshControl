package cmd

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/alexeiev/sshControl/config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

// HostResult armazena o resultado da execução em um host
type HostResult struct {
	Host             string
	Success          bool
	Output           string
	Error            string
	ExitCode         int
	ShouldAutoCreate bool   // Indica se o host deve ser auto-criado
	Hostname         string // Hostname real para auto-criação
	Port             int    // Porta para auto-criação
}

// ConnectMultiple executa um comando em múltiplos hosts em paralelo
func ConnectMultiple(cfg *config.ConfigFile, configPath string, hostArgs []string, selectedUser *config.User, jumpHost *config.JumpHost, command string, proxyEnabled bool, askPassword bool, verbose bool) {
	// Determina o usuário efetivo
	effectiveUser := cfg.GetEffectiveUser(selectedUser)
	if effectiveUser == nil {
		fmt.Fprintf(os.Stderr, "Erro: Nenhum usuário configurado\n")
		os.Exit(1)
	}

	// Valida chaves SSH apenas do usuário efetivo
	config.ValidateEffectiveUserSSHKeys(effectiveUser)

	// Expande tags e arquivos texto para hosts
	expandedHosts, tagsFound, err := ResolveHostInputs(cfg, hostArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erro: %v\n", err)
		os.Exit(1)
	}
	if len(expandedHosts) == 0 {
		fmt.Fprintf(os.Stderr, "Erro: Nenhum host válido especificado\n")
		os.Exit(1)
	}
	hostArgs = expandedHosts

	// Obtém configuração de proxy uma vez
	proxyAddress, proxyPort, proxyConfigured := cfg.Config.GetProxyConfig()
	proxyActive := proxyEnabled && proxyConfigured

	if !proxyActive && proxyEnabled {
		fmt.Fprintf(os.Stderr, "⚠️  Aviso: Proxy solicitado mas não configurado no config.yaml\n\n")
	}

	fmt.Println()
	if len(tagsFound) > 0 {
		fmt.Printf("🏷️  Tags: %s\n", strings.Join(tagsFound, ", "))
	}
	fmt.Printf("🚀 Executando comando em %d host(s): %s\n", len(hostArgs), command)
	if jumpHost != nil {
		fmt.Printf("   via Jump Host: %s (%s@%s:%d)\n", jumpHost.Name, jumpHost.User, jumpHost.Host, jumpHost.Port)
	}
	fmt.Println()

	// Em modo múltiplos hosts, solicita senha apenas se -a for especificado
	// Isso evita interrupção em automações/loops
	password := ""
	if askPassword {
		// Flag -a foi especificada, solicita senha antecipadamente
		if len(effectiveUser.SSHKeys) == 0 {
			// Usuário sem chave configurada - senha é obrigatória
			fmt.Printf("Password for %s (será usada para todos os hosts): ", effectiveUser.Name)
		} else {
			// Usuário com chave configurada - senha como fallback
			fmt.Printf("Password for %s (fallback caso chave SSH falhe, Enter para pular): ", effectiveUser.Name)
		}

		passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erro ao ler senha: %v\n", err)
			os.Exit(1)
		}
		password = string(passwordBytes)
		fmt.Println()
	}

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
			result := executeOnHost(cfg, hostArg, effectiveUser, jumpHost, password, command, proxyActive, proxyAddress, proxyPort, askPassword, verbose)
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

	// Auto-criação de hosts após execução bem-sucedida
	if cfg.Config.AutoCreate {
		autoCreateHostsFromResults(cfg, configPath, allResults)
	}
}

// executeOnHost executa o comando em um único host e retorna o resultado
func executeOnHost(cfg *config.ConfigFile, hostArg string, effectiveUser *config.User, jumpHost *config.JumpHost, password string, command string, proxyEnabled bool, proxyAddress string, proxyPort int, askPassword bool, verbose bool) HostResult {
	var hostname string
	var port int
	var sshKeys []string
	var shouldAutoCreate bool

	username := effectiveUser.Name
	for _, key := range effectiveUser.SSHKeys {
		sshKeys = append(sshKeys, config.ExpandHomePath(key))
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
			// Tenta obter as chaves SSH desse usuário específico
			if userFromConfig := cfg.FindUser(username); userFromConfig != nil {
				sshKeys = nil // Limpa chaves anteriores
				for _, key := range userFromConfig.SSHKeys {
					sshKeys = append(sshKeys, config.ExpandHomePath(key))
				}
			} else {
				sshKeys = nil
			}
		}

		hostname = host.hostname
		port = host.port

		// Verifica se auto_create está habilitado e se o host não existe pelo endereço
		if cfg.Config.AutoCreate && cfg.FindHostByAddress(hostname) == nil {
			shouldAutoCreate = true
		}
	}

	// Busca as chaves SSH do jump host se estiver usando jump host
	var jumpHostSSHKeys []string
	if jumpHost != nil {
		jumpHostSSHKeys = cfg.GetJumpHostSSHKeys(jumpHost)
	}

	// Cria a conexão SSH
	sshConn := NewSSHConnection(
		username,
		hostname,
		port,
		sshKeys,
		password, // Senha pré-fornecida ou vazia
		jumpHost,
		jumpHostSSHKeys,
		command,
		proxyEnabled,
		proxyAddress,
		proxyPort,
		verbose,
	)

	// Em modo múltiplos hosts, desabilita prompt interativo de senha
	// A senha já foi solicitada uma vez antes das conexões paralelas
	sshConn.InteractivePasswordAllowed = false

	// Executa o comando e captura a saída
	output, exitCode, err := sshConn.ExecuteCommandWithOutput()
	if err != nil {
		errorMsg := err.Error()

		// Se falhou por autenticação e não foi pedida senha (-a), sugere usar a flag
		if !askPassword && password == "" && len(sshKeys) == 0 {
			errorMsg += " (DICA: Use a opção -a ou --ask-password para fornecer senha)"
		} else if !askPassword && password == "" && len(sshKeys) > 0 {
			// Tem chave configurada mas pode não estar instalada
			errorMsg += " (DICA: Se a chave SSH não estiver instalada, use -a para fornecer senha)"
		}

		return HostResult{
			Host:     hostArg,
			Success:  false,
			Output:   output,
			Error:    errorMsg,
			ExitCode: exitCode,
		}
	}

	return HostResult{
		Host:             hostArg,
		Success:          true,
		Output:           output,
		ExitCode:         exitCode,
		ShouldAutoCreate: shouldAutoCreate,
		Hostname:         hostname,
		Port:             port,
	}
}

// autoCreateHostsFromResults adiciona hosts não cadastrados ao arquivo de configuração
func autoCreateHostsFromResults(cfg *config.ConfigFile, configPath string, results []HostResult) {
	var hostsToCreate []HostResult

	// Coleta hosts que devem ser auto-criados (apenas os bem-sucedidos)
	for _, result := range results {
		if result.Success && result.ShouldAutoCreate {
			hostsToCreate = append(hostsToCreate, result)
		}
	}

	if len(hostsToCreate) == 0 {
		return
	}

	// Adiciona os hosts à configuração
	for _, result := range hostsToCreate {
		newHost := config.Host{
			Name: result.Host,
			Host: result.Hostname,
			Port: result.Port,
			Tags: []string{"autocreated"},
		}
		cfg.AddHost(newHost)
	}

	// Salva a configuração
	if err := cfg.SaveConfig(configPath); err != nil {
		fmt.Fprintf(os.Stderr, "\n⚠️  Aviso: Não foi possível salvar os hosts no config.yaml: %v\n", err)
		return
	}

	// Exibe mensagem informativa
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("✅ %d host(s) adicionado(s) automaticamente ao config.yaml:\n", len(hostsToCreate))
	for _, result := range hostsToCreate {
		fmt.Printf("   - %s (%s:%d) [autocreated]\n", result.Host, result.Hostname, result.Port)
	}
	fmt.Println()
	fmt.Println("📝 Finalize a configuração dos hosts editando o arquivo:")
	fmt.Printf("   %s\n", configPath)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
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
	s.debugLog("Iniciando execução com captura de saída")
	s.debugLog("Host: %s:%d | Comando: %s", s.Host, s.Port, s.Command)

	// Cria a configuração SSH
	s.debugLog("Criando configuração SSH...")
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
	s.debugLog("Conexão SSH estabelecida com sucesso")

	// Tenta instalar a chave pública se necessário (não bloqueia em caso de erro)
	_ = s.installPublicKeyIfNeeded(client)

	// Cria uma sessão SSH
	s.debugLog("Criando sessão SSH...")
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
	s.debugLog("Executando comando...")
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
			s.debugLog("Comando encerrado com exit code: %d", exitCode)
			// Se temos um exit code, não é um erro de conexão
			return combinedOutput, exitCode, nil
		}
		return combinedOutput, -1, fmt.Errorf("erro ao executar comando: %w", err)
	}

	s.debugLog("Comando executado com sucesso (exit code: 0)")
	return combinedOutput, exitCode, nil
}

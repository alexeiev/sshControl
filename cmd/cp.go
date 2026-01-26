package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alexeiev/sshControl/config"
	"github.com/pkg/sftp"
)

// TransferResult armazena o resultado de uma transferência
type TransferResult struct {
	Host      string
	Success   bool
	FilePath  string
	BytesSent int64
	Duration  time.Duration
	Error     string
}

// FileTransfer gerencia transferências de arquivos via SFTP
type FileTransfer struct {
	LocalPath  string
	RemotePath string
	Recursive  bool
}

// ProgressWriter implementa io.Writer para exibir progresso
type ProgressWriter struct {
	Total     int64
	Written   int64
	Filename  string
	Host      string
	StartTime time.Time
	lastPrint time.Time
}

// NewProgressWriter cria um novo ProgressWriter
func NewProgressWriter(filename string, host string, total int64) *ProgressWriter {
	return &ProgressWriter{
		Total:     total,
		Filename:  filename,
		Host:      host,
		StartTime: time.Now(),
		lastPrint: time.Now(),
	}
}

// Write implementa io.Writer e atualiza o progresso
func (pw *ProgressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.Written += int64(n)

	// Atualiza a exibição no máximo a cada 100ms para não sobrecarregar o terminal
	if time.Since(pw.lastPrint) >= 100*time.Millisecond || pw.Written >= pw.Total {
		pw.printProgress()
		pw.lastPrint = time.Now()
	}

	return n, nil
}

// printProgress exibe a barra de progresso
func (pw *ProgressWriter) printProgress() {
	percent := float64(pw.Written) / float64(pw.Total) * 100
	if pw.Total == 0 {
		percent = 100
	}

	// Calcula a barra de progresso (20 caracteres)
	barWidth := 20
	filled := int(percent / 100 * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	bar := strings.Repeat("=", filled)
	if filled < barWidth && filled > 0 {
		bar += ">"
		bar += strings.Repeat(" ", barWidth-filled-1)
	} else if filled < barWidth {
		bar += strings.Repeat(" ", barWidth-filled)
	}

	// Formata os bytes
	writtenStr := formatBytes(pw.Written)
	totalStr := formatBytes(pw.Total)

	// Imprime na mesma linha (usando \r)
	fmt.Printf("\r%s: %s... %3.0f%% [%s] %s/%s", pw.Host, pw.Filename, percent, bar, writtenStr, totalStr)
}

// Finish finaliza a exibição do progresso
func (pw *ProgressWriter) Finish() {
	duration := time.Since(pw.StartTime)
	totalStr := formatBytes(pw.Written)
	// Limpa a linha e exibe resultado final
	fmt.Printf("\r%s: %s (%s em %.1fs)                                    \n", pw.Host, pw.Filename, totalStr, duration.Seconds())
}

// formatBytes formata bytes para exibição legível
func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1fGB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1fMB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1fKB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

// Download baixa um arquivo ou diretório do servidor remoto
func (ft *FileTransfer) Download(sshConn *SSHConnection) error {
	// Cria a configuração SSH
	sshConfig, err := sshConn.createSSHConfig()
	if err != nil {
		return fmt.Errorf("erro ao criar configuração SSH: %w", err)
	}

	// Conecta ao host
	client, err := sshConn.dial(sshConfig)
	if err != nil {
		return fmt.Errorf("erro ao conectar: %w", err)
	}
	defer client.Close()

	// Cria cliente SFTP
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("erro ao criar cliente SFTP: %w", err)
	}
	defer sftpClient.Close()

	// Expande ~ para o diretório home do usuário remoto
	remotePath := expandRemotePath(sftpClient, ft.RemotePath)

	// Verifica se é arquivo ou diretório
	remoteInfo, err := sftpClient.Stat(remotePath)
	if err != nil {
		return fmt.Errorf("erro ao acessar '%s': %w", remotePath, err)
	}

	hostLabel := fmt.Sprintf("%s@%s", sshConn.User, sshConn.Host)

	if remoteInfo.IsDir() {
		if !ft.Recursive {
			return fmt.Errorf("'%s' é um diretório. Use -r para copiar recursivamente", remotePath)
		}
		return ft.downloadDir(sftpClient, remotePath, ft.LocalPath, hostLabel)
	}

	return ft.downloadFile(sftpClient, remotePath, ft.LocalPath, hostLabel)
}

// downloadFile baixa um único arquivo
func (ft *FileTransfer) downloadFile(sftpClient *sftp.Client, remotePath, localPath, hostLabel string) error {
	// Abre arquivo remoto
	remoteFile, err := sftpClient.Open(remotePath)
	if err != nil {
		return fmt.Errorf("erro ao abrir arquivo remoto: %w", err)
	}
	defer remoteFile.Close()

	// Obtém informações do arquivo
	remoteInfo, err := remoteFile.Stat()
	if err != nil {
		return fmt.Errorf("erro ao obter informações do arquivo: %w", err)
	}

	// Determina o caminho local
	destPath := localPath
	localInfo, err := os.Stat(localPath)
	if err == nil && localInfo.IsDir() {
		// Se destino é diretório, usa o mesmo nome do arquivo remoto
		destPath = filepath.Join(localPath, filepath.Base(remotePath))
	} else if os.IsNotExist(err) {
		// Se não existe, verifica se o pai existe
		parentDir := filepath.Dir(localPath)
		if _, err := os.Stat(parentDir); os.IsNotExist(err) {
			return fmt.Errorf("diretório pai '%s' não existe", parentDir)
		}
	}

	// Cria arquivo local
	localFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("erro ao criar arquivo local: %w", err)
	}
	defer localFile.Close()

	// Cria progress writer
	pw := NewProgressWriter(filepath.Base(remotePath), hostLabel, remoteInfo.Size())

	// Copia com progresso
	_, err = io.Copy(io.MultiWriter(localFile, pw), remoteFile)
	if err != nil {
		return fmt.Errorf("erro ao copiar arquivo: %w", err)
	}

	pw.Finish()
	return nil
}

// downloadDir baixa um diretório recursivamente
func (ft *FileTransfer) downloadDir(sftpClient *sftp.Client, remotePath, localPath, hostLabel string) error {
	// Cria diretório local se não existir
	if err := os.MkdirAll(localPath, 0755); err != nil {
		return fmt.Errorf("erro ao criar diretório local: %w", err)
	}

	// Lista arquivos do diretório remoto
	entries, err := sftpClient.ReadDir(remotePath)
	if err != nil {
		return fmt.Errorf("erro ao listar diretório remoto: %w", err)
	}

	for _, entry := range entries {
		remoteEntryPath := filepath.Join(remotePath, entry.Name())
		localEntryPath := filepath.Join(localPath, entry.Name())

		if entry.IsDir() {
			if err := ft.downloadDir(sftpClient, remoteEntryPath, localEntryPath, hostLabel); err != nil {
				return err
			}
		} else {
			if err := ft.downloadFile(sftpClient, remoteEntryPath, localEntryPath, hostLabel); err != nil {
				return err
			}
		}
	}

	return nil
}

// Upload envia um arquivo ou diretório para o servidor remoto
func (ft *FileTransfer) Upload(sshConn *SSHConnection) error {
	// Verifica se arquivo/diretório local existe
	localInfo, err := os.Stat(ft.LocalPath)
	if err != nil {
		return fmt.Errorf("erro ao acessar '%s': %w", ft.LocalPath, err)
	}

	// Cria a configuração SSH
	sshConfig, err := sshConn.createSSHConfig()
	if err != nil {
		return fmt.Errorf("erro ao criar configuração SSH: %w", err)
	}

	// Conecta ao host
	client, err := sshConn.dial(sshConfig)
	if err != nil {
		return fmt.Errorf("erro ao conectar: %w", err)
	}
	defer client.Close()

	// Cria cliente SFTP
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("erro ao criar cliente SFTP: %w", err)
	}
	defer sftpClient.Close()

	hostLabel := fmt.Sprintf("%s@%s", sshConn.User, sshConn.Host)

	if localInfo.IsDir() {
		if !ft.Recursive {
			return fmt.Errorf("'%s' é um diretório. Use -r para copiar recursivamente", ft.LocalPath)
		}
		return ft.uploadDir(sftpClient, ft.LocalPath, ft.RemotePath, hostLabel)
	}

	return ft.uploadFile(sftpClient, ft.LocalPath, ft.RemotePath, hostLabel)
}

// expandRemotePath expande ~ para o diretório home do usuário remoto
// Também detecta e corrige quando o shell local expandiu ~ para o home local
func expandRemotePath(sftpClient *sftp.Client, remotePath string) string {
	// Caso 1: ~ ou ~/ não foi expandido pelo shell (usuário usou aspas)
	if remotePath == "~" || strings.HasPrefix(remotePath, "~/") {
		homeDir, err := sftpClient.Getwd()
		if err != nil {
			homeDir = "."
		}
		if remotePath == "~" {
			return homeDir
		}
		return filepath.Join(homeDir, remotePath[2:])
	}

	// Caso 2: Shell expandiu ~ para o home local - precisamos corrigir
	localHome, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(remotePath, localHome) {
		// Obtém o home remoto
		remoteHome, err := sftpClient.Getwd()
		if err != nil {
			remoteHome = "."
		}
		// Substitui o home local pelo home remoto
		relativePath := strings.TrimPrefix(remotePath, localHome)
		return filepath.Join(remoteHome, relativePath)
	}

	return remotePath
}

// uploadFile envia um único arquivo
func (ft *FileTransfer) uploadFile(sftpClient *sftp.Client, localPath, remotePath, hostLabel string) error {
	// Abre arquivo local
	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("erro ao abrir arquivo local: %w", err)
	}
	defer localFile.Close()

	// Obtém informações do arquivo
	localInfo, err := localFile.Stat()
	if err != nil {
		return fmt.Errorf("erro ao obter informações do arquivo: %w", err)
	}

	// Expande ~ para o diretório home do usuário remoto
	remotePath = expandRemotePath(sftpClient, remotePath)

	// Determina o caminho remoto
	destPath := remotePath
	remoteInfo, err := sftpClient.Stat(remotePath)
	if err == nil && remoteInfo.IsDir() {
		// Se destino é diretório, usa o mesmo nome do arquivo local
		destPath = filepath.Join(remotePath, filepath.Base(localPath))
	}

	// Cria arquivo remoto
	remoteFile, err := sftpClient.Create(destPath)
	if err != nil {
		return fmt.Errorf("erro ao criar arquivo remoto '%s': %w", destPath, err)
	}
	defer remoteFile.Close()

	// Cria progress writer
	pw := NewProgressWriter(filepath.Base(localPath), hostLabel, localInfo.Size())

	// Copia com progresso
	_, err = io.Copy(io.MultiWriter(remoteFile, pw), localFile)
	if err != nil {
		return fmt.Errorf("erro ao copiar arquivo: %w", err)
	}

	pw.Finish()
	return nil
}

// uploadDir envia um diretório recursivamente
func (ft *FileTransfer) uploadDir(sftpClient *sftp.Client, localPath, remotePath, hostLabel string) error {
	// Expande ~ para o diretório home do usuário remoto
	remotePath = expandRemotePath(sftpClient, remotePath)

	// Determina o caminho remoto do diretório
	destPath := remotePath
	remoteInfo, err := sftpClient.Stat(remotePath)
	if err == nil && remoteInfo.IsDir() {
		// Se destino é diretório existente, cria subdiretório com nome do local
		destPath = filepath.Join(remotePath, filepath.Base(localPath))
	}

	// Cria diretório remoto
	if err := sftpClient.MkdirAll(destPath); err != nil {
		return fmt.Errorf("erro ao criar diretório remoto '%s': %w", destPath, err)
	}

	// Lista arquivos do diretório local
	entries, err := os.ReadDir(localPath)
	if err != nil {
		return fmt.Errorf("erro ao listar diretório local: %w", err)
	}

	for _, entry := range entries {
		localEntryPath := filepath.Join(localPath, entry.Name())
		remoteEntryPath := filepath.Join(destPath, entry.Name())

		if entry.IsDir() {
			// Para subdiretórios, passamos o caminho direto sem adicionar basename novamente
			subFt := &FileTransfer{
				LocalPath:  localEntryPath,
				RemotePath: remoteEntryPath,
				Recursive:  true,
			}
			if err := subFt.uploadDirRecursive(sftpClient, localEntryPath, remoteEntryPath, hostLabel); err != nil {
				return err
			}
		} else {
			if err := ft.uploadFile(sftpClient, localEntryPath, remoteEntryPath, hostLabel); err != nil {
				return err
			}
		}
	}

	return nil
}

// uploadDirRecursive é uma versão interna que não adiciona basename
func (ft *FileTransfer) uploadDirRecursive(sftpClient *sftp.Client, localPath, remotePath, hostLabel string) error {
	// Cria diretório remoto
	if err := sftpClient.MkdirAll(remotePath); err != nil {
		return fmt.Errorf("erro ao criar diretório remoto '%s': %w", remotePath, err)
	}

	// Lista arquivos do diretório local
	entries, err := os.ReadDir(localPath)
	if err != nil {
		return fmt.Errorf("erro ao listar diretório local: %w", err)
	}

	for _, entry := range entries {
		localEntryPath := filepath.Join(localPath, entry.Name())
		remoteEntryPath := filepath.Join(remotePath, entry.Name())

		if entry.IsDir() {
			if err := ft.uploadDirRecursive(sftpClient, localEntryPath, remoteEntryPath, hostLabel); err != nil {
				return err
			}
		} else {
			if err := ft.uploadFile(sftpClient, localEntryPath, remoteEntryPath, hostLabel); err != nil {
				return err
			}
		}
	}

	return nil
}

// UploadMultiple envia arquivo para múltiplos hosts em paralelo
func (ft *FileTransfer) UploadMultiple(cfg *config.ConfigFile, hostArgs []string, effectiveUser *config.User, jumpHost *config.JumpHost, password string, askPassword bool) []TransferResult {
	// Expande tags para hosts
	expandedHosts, tagsFound := expandTagsToHosts(cfg, hostArgs)
	if len(tagsFound) > 0 {
		fmt.Printf("Tags: %s\n", strings.Join(tagsFound, ", "))
	}

	results := make(chan TransferResult, len(expandedHosts))
	var wg sync.WaitGroup

	for _, hostArg := range expandedHosts {
		wg.Add(1)
		go func(hostArg string) {
			defer wg.Done()
			result := ft.uploadToHost(cfg, hostArg, effectiveUser, jumpHost, password)
			results <- result
		}(hostArg)
	}

	// Aguarda todas as goroutines
	go func() {
		wg.Wait()
		close(results)
	}()

	// Coleta resultados
	var allResults []TransferResult
	for result := range results {
		allResults = append(allResults, result)
	}

	return allResults
}

// uploadToHost envia arquivo para um único host
func (ft *FileTransfer) uploadToHost(cfg *config.ConfigFile, hostArg string, effectiveUser *config.User, jumpHost *config.JumpHost, password string) TransferResult {
	startTime := time.Now()

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
			return TransferResult{
				Host:    hostArg,
				Success: false,
				Error:   fmt.Sprintf("Formato inválido: %v", err),
			}
		}

		if host.parsedUser != "" && host.parsedUser != effectiveUser.Name {
			username = host.parsedUser
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

	// Busca a chave SSH do jump host
	jumpHostSSHKey := ""
	if jumpHost != nil {
		jumpHostSSHKey = cfg.GetJumpHostSSHKey(jumpHost)
	}

	// Cria a conexão SSH
	sshConn := NewSSHConnection(
		username,
		hostname,
		port,
		sshKey,
		password,
		jumpHost,
		jumpHostSSHKey,
		"",    // sem comando
		false, // sem proxy
		"",
		0,
	)
	sshConn.InteractivePasswordAllowed = false

	// Verifica arquivo local
	localInfo, err := os.Stat(ft.LocalPath)
	if err != nil {
		return TransferResult{
			Host:    hostArg,
			Success: false,
			Error:   fmt.Sprintf("Erro ao acessar arquivo local: %v", err),
		}
	}

	// Executa o upload
	err = ft.Upload(sshConn)
	duration := time.Since(startTime)

	if err != nil {
		return TransferResult{
			Host:     hostArg,
			Success:  false,
			Error:    err.Error(),
			Duration: duration,
		}
	}

	return TransferResult{
		Host:      hostArg,
		Success:   true,
		FilePath:  ft.LocalPath,
		BytesSent: localInfo.Size(),
		Duration:  duration,
	}
}

// DisplayTransferResults exibe os resultados das transferências
func DisplayTransferResults(results []TransferResult, totalDuration time.Duration) {
	successCount := 0
	failureCount := 0

	fmt.Println()
	fmt.Println("------------------------------------------------------------------------------------")

	for _, result := range results {
		if result.Success {
			successCount++
			fmt.Printf(" Host: %s (%.1fs)\n", result.Host, result.Duration.Seconds())
		} else {
			failureCount++
			fmt.Printf(" Host: %s - Erro: %s\n", result.Host, result.Error)
		}
	}

	fmt.Println("------------------------------------------------------------------------------------")
	fmt.Printf(" Resumo: %d sucesso(s), %d falha(s), %d total | Tempo: %.2fs\n",
		successCount, failureCount, len(results), totalDuration.Seconds())
	fmt.Println("------------------------------------------------------------------------------------")
}

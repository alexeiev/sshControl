package updater

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	githubAPIURL = "https://api.github.com/repos/%s/%s/releases/latest"
	repoOwner    = "alexeiev"
	repoName     = "sshControl"
	timeout      = 30 * time.Second
)

// Release representa uma release do GitHub
type Release struct {
	TagName    string  `json:"tag_name"`
	Name       string  `json:"name"`
	Body       string  `json:"body"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}

// Asset representa um arquivo anexado a uma release
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int    `json:"size"`
}

// Updater gerencia atualizações da ferramenta
type Updater struct {
	CurrentVersion string
	RepoOwner      string
	RepoName       string
}

// New cria um novo Updater
func New(currentVersion string) *Updater {
	return &Updater{
		CurrentVersion: currentVersion,
		RepoOwner:      repoOwner,
		RepoName:       repoName,
	}
}

// CheckForUpdates verifica se há uma nova versão disponível
func (u *Updater) CheckForUpdates() (*Release, bool, error) {
	url := fmt.Sprintf(githubAPIURL, u.RepoOwner, u.RepoName)

	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, false, fmt.Errorf("erro ao consultar GitHub API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("GitHub API retornou status %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, false, fmt.Errorf("erro ao decodificar resposta: %w", err)
	}

	// Ignora draft e pre-releases
	if release.Draft || release.Prerelease {
		return nil, false, nil
	}

	// Compara versões
	hasUpdate := u.compareVersions(u.CurrentVersion, release.TagName)
	return &release, hasUpdate, nil
}

// Update baixa e instala a nova versão
func (u *Updater) Update(release *Release) error {
	// Determina qual asset baixar baseado em OS e arquitetura
	assetName := u.getAssetName(release.TagName)

	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("asset não encontrado para %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	fmt.Printf("Baixando %s...\n", assetName)

	// Baixa o arquivo
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("erro ao baixar: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("erro ao baixar: status %d", resp.StatusCode)
	}

	// Cria arquivo temporário
	tmpFile, err := os.CreateTemp("", "sc-update-*.tar.gz")
	if err != nil {
		return fmt.Errorf("erro ao criar arquivo temporário: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Salva o download
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return fmt.Errorf("erro ao salvar download: %w", err)
	}

	// Extrai o binário
	if _, err := tmpFile.Seek(0, 0); err != nil {
		return fmt.Errorf("erro ao reposicionar arquivo: %w", err)
	}

	newBinaryPath, err := u.extractBinary(tmpFile)
	if err != nil {
		return fmt.Errorf("erro ao extrair binário: %w", err)
	}
	defer os.Remove(newBinaryPath)

	// Substitui o binário atual
	if err := u.replaceBinary(newBinaryPath); err != nil {
		return fmt.Errorf("erro ao substituir binário: %w", err)
	}

	fmt.Println("✅ Atualização concluída com sucesso!")
	fmt.Printf("Nova versão: %s\n", release.TagName)

	return nil
}

// extractBinary extrai o binário do arquivo tar.gz
func (u *Updater) extractBinary(file *os.File) (string, error) {
	gzr, err := gzip.NewReader(file)
	if err != nil {
		return "", fmt.Errorf("erro ao descompactar gzip: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("erro ao ler tar: %w", err)
		}

		// Procura pelo arquivo 'sc'
		if header.Name == "sc" && header.Typeflag == tar.TypeReg {
			tmpBinary, err := os.CreateTemp("", "sc-new-*")
			if err != nil {
				return "", fmt.Errorf("erro ao criar arquivo temporário: %w", err)
			}
			defer tmpBinary.Close()

			if _, err := io.Copy(tmpBinary, tr); err != nil {
				os.Remove(tmpBinary.Name())
				return "", fmt.Errorf("erro ao copiar binário: %w", err)
			}

			// Define permissões executáveis
			if err := os.Chmod(tmpBinary.Name(), 0755); err != nil {
				os.Remove(tmpBinary.Name())
				return "", fmt.Errorf("erro ao definir permissões: %w", err)
			}

			return tmpBinary.Name(), nil
		}
	}

	return "", fmt.Errorf("binário 'sc' não encontrado no arquivo")
}

// replaceBinary substitui o binário atual pelo novo
func (u *Updater) replaceBinary(newBinaryPath string) error {
	// Obtém o caminho do binário atual
	currentBinaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("erro ao obter caminho do executável: %w", err)
	}

	// Resolve symlinks
	currentBinaryPath, err = filepath.EvalSymlinks(currentBinaryPath)
	if err != nil {
		return fmt.Errorf("erro ao resolver symlinks: %w", err)
	}

	// Verifica se temos permissão de escrita no diretório
	dir := filepath.Dir(currentBinaryPath)
	if !hasWritePermission(dir) {
		return fmt.Errorf("permissão negada para atualizar %s\n\nPara atualizar, execute com sudo:\n  sudo sc update", currentBinaryPath)
	}

	// Cria backup do binário atual
	backupPath := currentBinaryPath + ".backup"
	if err := os.Rename(currentBinaryPath, backupPath); err != nil {
		// Verifica se é erro de permissão
		if os.IsPermission(err) {
			return fmt.Errorf("permissão negada para atualizar %s\n\nPara atualizar, execute com sudo:\n  sudo sc update", currentBinaryPath)
		}
		return fmt.Errorf("erro ao criar backup: %w", err)
	}

	// Copia o novo binário para o local do atual
	if err := copyFile(newBinaryPath, currentBinaryPath); err != nil {
		// Tenta restaurar backup em caso de erro
		os.Rename(backupPath, currentBinaryPath)
		return fmt.Errorf("erro ao copiar novo binário: %w", err)
	}

	// Define permissões executáveis
	if err := os.Chmod(currentBinaryPath, 0755); err != nil {
		return fmt.Errorf("erro ao definir permissões: %w", err)
	}

	// Remove backup se tudo correu bem
	os.Remove(backupPath)

	return nil
}

// hasWritePermission verifica se temos permissão de escrita no diretório
func hasWritePermission(dir string) bool {
	// Tenta criar um arquivo temporário no diretório
	testFile := filepath.Join(dir, ".sc-write-test")
	f, err := os.Create(testFile)
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(testFile)
	return true
}

// copyFile copia um arquivo
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// getAssetName retorna o nome do asset baseado no OS e arquitetura
func (u *Updater) getAssetName(version string) string {
	return fmt.Sprintf("sc-%s-%s-%s.tar.gz", version, runtime.GOOS, runtime.GOARCH)
}

// compareVersions compara duas versões e retorna true se newVersion > currentVersion
func (u *Updater) compareVersions(current, latest string) bool {
	// Remove 'v' prefix se presente
	current = strings.TrimPrefix(current, "v")
	latest = strings.TrimPrefix(latest, "v")

	// Trata versão "dev" como antiga
	if current == "dev" {
		return true
	}

	// Comparação simples de strings (funciona para semver básico)
	return latest > current
}

package cmd

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/alexeiev/sshControl/config"
)

var hostListEntryPattern = regexp.MustCompile(`^(?:[^@\s/:]+@)?[A-Za-z0-9._-]+(?::\d+)?$`)

// ResolveHostInputs expande entradas de host para aceitar hosts diretos, tags e arquivos texto.
// Quando mais de uma tag é informada, apenas os hosts que possuem TODAS as tags são incluídos (interseção).
// Hosts informados diretamente são sempre incluídos.
func ResolveHostInputs(cfg *config.ConfigFile, hostArgs []string) ([]string, []string, error) {
	var expandedHosts []string
	var tagsFound []string

	hostSet := make(map[string]bool)
	tagSet := make(map[string]bool)

	// Posição em expandedHosts onde os hosts das tags serão inseridos (-1 = nenhuma tag)
	tagInsertPos := -1

	for _, arg := range hostArgs {
		if err := appendHostInput(cfg, strings.TrimSpace(arg), true, hostSet, tagSet, &expandedHosts, &tagsFound, &tagInsertPos); err != nil {
			return nil, nil, err
		}
	}

	if len(tagsFound) == 0 {
		return expandedHosts, tagsFound, nil
	}

	tagHosts := cfg.FindHostsByTags(tagsFound)
	if len(tagHosts) == 0 {
		fmt.Fprintf(os.Stderr, "⚠️  Aviso: %s\n", noHostsForTagsMessage(tagsFound))
		return expandedHosts, tagsFound, nil
	}

	var tagHostNames []string
	for _, host := range tagHosts {
		if !hostSet[host.Name] {
			hostSet[host.Name] = true
			tagHostNames = append(tagHostNames, host.Name)
		}
	}

	result := make([]string, 0, len(expandedHosts)+len(tagHostNames))
	result = append(result, expandedHosts[:tagInsertPos]...)
	result = append(result, tagHostNames...)
	result = append(result, expandedHosts[tagInsertPos:]...)

	return result, tagsFound, nil
}

// noHostsForTagsMessage monta a mensagem de aviso quando nenhum host possui as tags informadas
func noHostsForTagsMessage(tags []string) string {
	if len(tags) == 1 {
		return fmt.Sprintf("Nenhum host encontrado com a tag '%s'", tags[0])
	}
	return fmt.Sprintf("Nenhum host encontrado com todas as tags '%s'", strings.Join(tags, "', '"))
}

// ParseMultipleUploadArgs identifica os hosts, o arquivo local e o destino remoto no modo `cp up -l`.
func ParseMultipleUploadArgs(cfg *config.ConfigFile, args []string) ([]string, string, string, error) {
	localIdx := -1

	for i, arg := range args {
		info, err := os.Stat(arg)
		if err != nil {
			continue
		}

		if info.Mode().IsRegular() && IsHostListFile(cfg, arg) {
			continue
		}

		localIdx = i
		break
	}

	if localIdx == -1 {
		return nil, "", "", fmt.Errorf("nenhum arquivo local válido encontrado nos argumentos")
	}

	hostArgs := args[:localIdx]
	if len(hostArgs) == 0 {
		return nil, "", "", fmt.Errorf("nenhum host especificado")
	}

	localPath := args[localIdx]
	remotePath := "~"
	if localIdx+1 < len(args) {
		remotePath = args[localIdx+1]
	}

	return hostArgs, localPath, remotePath, nil
}

// IsHostListFile verifica se o caminho aponta para um arquivo texto contendo hosts/tags.
func IsHostListFile(cfg *config.ConfigFile, path string) bool {
	if cfg != nil && cfg.FindHost(path) != nil {
		return false
	}

	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}

	entries, err := readHostListFile(path)
	if err != nil || len(entries) == 0 {
		return false
	}

	for _, entry := range entries {
		if isKnownHostListEntry(cfg, entry) {
			continue
		}
		return false
	}

	return true
}

func appendHostInput(cfg *config.ConfigFile, arg string, allowFile bool, hostSet map[string]bool, tagSet map[string]bool, expandedHosts *[]string, tagsFound *[]string, tagInsertPos *int) error {
	if arg == "" {
		return nil
	}

	if allowFile && IsHostListFile(cfg, arg) {
		fileHosts, err := readHostListFile(arg)
		if err != nil {
			return fmt.Errorf("erro ao ler arquivo de hosts '%s': %w", arg, err)
		}

		for _, fileHost := range fileHosts {
			if err := appendHostInput(cfg, fileHost, false, hostSet, tagSet, expandedHosts, tagsFound, tagInsertPos); err != nil {
				return err
			}
		}
		return nil
	}

	// Tags são apenas coletadas aqui; a interseção é resolvida em ResolveHostInputs
	if strings.HasPrefix(arg, "@") {
		tag := strings.TrimPrefix(arg, "@")
		if tag == "" {
			return nil
		}

		if *tagInsertPos < 0 {
			*tagInsertPos = len(*expandedHosts)
		}

		tagKey := strings.ToLower(tag)
		if !tagSet[tagKey] {
			tagSet[tagKey] = true
			*tagsFound = append(*tagsFound, tag)
		}
		return nil
	}

	if !hostSet[arg] {
		hostSet[arg] = true
		*expandedHosts = append(*expandedHosts, arg)
	}

	return nil
}

func readHostListFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return splitHostEntries(string(data)), nil
}

func splitHostEntries(content string) []string {
	rawEntries := strings.FieldsFunc(content, func(r rune) bool {
		switch r {
		case ',', ';', '\n', '\r':
			return true
		default:
			return false
		}
	})

	entries := make([]string, 0, len(rawEntries))
	for _, entry := range rawEntries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		entries = append(entries, entry)
	}

	return entries
}

func isKnownHostListEntry(cfg *config.ConfigFile, entry string) bool {
	if strings.HasPrefix(entry, "@") {
		return len(strings.TrimPrefix(entry, "@")) > 0
	}

	if cfg != nil && cfg.FindHost(entry) != nil {
		return true
	}

	return hostListEntryPattern.MatchString(entry)
}

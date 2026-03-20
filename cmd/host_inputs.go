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
func ResolveHostInputs(cfg *config.ConfigFile, hostArgs []string) ([]string, []string, error) {
	var expandedHosts []string
	var tagsFound []string

	hostSet := make(map[string]bool)
	tagSet := make(map[string]bool)

	for _, arg := range hostArgs {
		if err := appendHostInput(cfg, strings.TrimSpace(arg), true, hostSet, tagSet, &expandedHosts, &tagsFound); err != nil {
			return nil, nil, err
		}
	}

	return expandedHosts, tagsFound, nil
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

func appendHostInput(cfg *config.ConfigFile, arg string, allowFile bool, hostSet map[string]bool, tagSet map[string]bool, expandedHosts *[]string, tagsFound *[]string) error {
	if arg == "" {
		return nil
	}

	if allowFile && IsHostListFile(cfg, arg) {
		fileHosts, err := readHostListFile(arg)
		if err != nil {
			return fmt.Errorf("erro ao ler arquivo de hosts '%s': %w", arg, err)
		}

		for _, fileHost := range fileHosts {
			if err := appendHostInput(cfg, fileHost, false, hostSet, tagSet, expandedHosts, tagsFound); err != nil {
				return err
			}
		}
		return nil
	}

	if strings.HasPrefix(arg, "@") {
		tag := strings.TrimPrefix(arg, "@")
		if tag == "" {
			return nil
		}

		if !tagSet[tag] {
			tagSet[tag] = true
			*tagsFound = append(*tagsFound, tag)
		}

		hosts := cfg.FindHostsByTag(tag)
		if len(hosts) == 0 {
			fmt.Fprintf(os.Stderr, "⚠️  Aviso: Nenhum host encontrado com a tag '%s'\n", tag)
			return nil
		}

		for _, host := range hosts {
			if !hostSet[host.Name] {
				hostSet[host.Name] = true
				*expandedHosts = append(*expandedHosts, host.Name)
			}
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

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/ceiev/sshControl/cmd"
	"github.com/ceiev/sshControl/config"
)

func main() {
	// Define as flags
	username := flag.String("u", "", "Nome do usuário da configuração a ser usado")
	useJumpHost := flag.Bool("s", false, "Habilita conexão via Jump Host")
	command := flag.String("c", "", "Comando a ser executado remotamente")
	flag.Parse()

	// Inicializa o diretório de configuração e obtém o caminho do arquivo
	configPath, err := config.InitializeConfigDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erro ao inicializar configuração: %v\n", err)
		os.Exit(1)
	}

	// Carrega o arquivo de configuração
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erro ao carregar %s: %v\n", configPath, err)
		fmt.Fprintf(os.Stderr, "Verifique se o arquivo está no formato correto.\n")
		os.Exit(1)
	}

	// Valida o Jump Host se solicitado
	if *useJumpHost && cfg.Config.JumpHosts == "" {
		fmt.Fprintf(os.Stderr, "Aviso: Jump Host solicitado mas não configurado no config.yaml\n")
		fmt.Fprintf(os.Stderr, "A opção -s será ignorada.\n\n")
		*useJumpHost = false
	}

	// Valida e aplica o usuário se especificado
	var selectedUser *config.User
	if *username != "" {
		selectedUser = cfg.FindUser(*username)
		if selectedUser == nil {
			fmt.Fprintf(os.Stderr, "Erro: Usuário '%s' não encontrado no config.yaml\n", *username)
			if len(cfg.Config.User) > 0 {
				fmt.Fprintf(os.Stderr, "Usuários disponíveis: ")
				for i, u := range cfg.Config.User {
					if i > 0 {
						fmt.Fprintf(os.Stderr, ", ")
					}
					fmt.Fprintf(os.Stderr, "%s", u.Name)
				}
				fmt.Fprintf(os.Stderr, "\n")
			}
			os.Exit(1)
		}
	}

	// Pega os argumentos restantes após as flags
	args := flag.Args()

	// Verifica se há argumentos (modo direto)
	if len(args) > 0 {
		hostArg := args[0]
		cmd.Connect(cfg, hostArg, selectedUser, *useJumpHost, *command)
		return
	}

	// Modo interativo não suporta execução de comando remoto
	if *command != "" {
		fmt.Fprintf(os.Stderr, "Erro: A opção -c requer especificar um host\n")
		fmt.Fprintf(os.Stderr, "Uso: sc -c \"comando\" <host>\n")
		os.Exit(1)
	}

	// Modo interativo (menu)
	cmd.ShowInteractive(cfg, selectedUser, *useJumpHost)
}

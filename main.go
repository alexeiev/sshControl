package main

import (
	"fmt"
	"os"

	"github.com/ceiev/sshControl/cmd"
	"github.com/ceiev/sshControl/config"
	"github.com/spf13/cobra"
)

var (
	username      string
	useJumpHost   bool
	command       string
	multipleHosts bool
	showServers   bool
)

var rootCmd = &cobra.Command{
	Use:   "sc [host]",
	Short: "sshControl - Gerenciador de conexões SSH",
	Long: `sshControl (sc) é um gerenciador de conexões SSH que oferece modos
interativo (TUI) e CLI direto para gerenciar conexões SSH.

Suporta conexões através de jump hosts, execução de comandos remotos
e gerenciamento de múltiplos hosts em paralelo.`,
	Example: `  # Modo interativo (menu TUI)
  sc
  sc -u ubuntu
  sc -j

  # Conexão direta
  sc webserver
  sc 192.168.1.50
  sc ubuntu@192.168.1.50:2222
  sc -j production-db

  # Executar comando remoto em host único
  sc -c "uptime" webserver
  sc -u deploy -c "systemctl status nginx" webserver
  sc -j -c "cat /var/log/app.log" production-app

  # Executar comando em múltiplos hosts
  sc -c "uptime" -l web1 web2 web3
  sc -c "free -h" -l 192.168.1.10 192.168.1.11
  sc -j -c "df -h" -l db1 db2 db3

  # Listar servidores cadastrados
  sc -s`,
	Run: runCommand,
}

func init() {
	rootCmd.Flags().StringVarP(&username, "user", "u", "", "Nome do usuário da configuração a ser usado")
	rootCmd.Flags().BoolVarP(&useJumpHost, "jump", "j", false, "Habilita conexão via Jump Host")
	rootCmd.Flags().StringVarP(&command, "command", "c", "", "Comando a ser executado remotamente")
	rootCmd.Flags().BoolVarP(&multipleHosts, "list", "l", false, "Executa comando em múltiplos hosts (requer -c)")
	rootCmd.Flags().BoolVarP(&showServers, "servers", "s", false, "Lista todos os servidores cadastrados no config")
}

func runCommand(cobraCmd *cobra.Command, args []string) {
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

	// Se a flag -s foi usada, lista os servidores e sai
	if showServers {
		cmd.ListServers(cfg)
		return
	}

	// Valida o Jump Host se solicitado
	if useJumpHost && cfg.Config.JumpHosts == "" {
		fmt.Fprintf(os.Stderr, "Aviso: Jump Host solicitado mas não configurado no config.yaml\n")
		fmt.Fprintf(os.Stderr, "A opção -j será ignorada.\n\n")
		useJumpHost = false
	}

	// Valida e aplica o usuário se especificado
	var selectedUser *config.User
	if username != "" {
		selectedUser = cfg.FindUser(username)
		if selectedUser == nil {
			fmt.Fprintf(os.Stderr, "Erro: Usuário '%s' não encontrado no config.yaml\n", username)
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

	// Validação: -l requer -c
	if multipleHosts && command == "" {
		fmt.Fprintf(os.Stderr, "Erro: A opção -l requer especificar um comando com -c\n")
		fmt.Fprintf(os.Stderr, "Uso: sc -c \"comando\" -l <host1> <host2> <host3> ...\n")
		os.Exit(1)
	}

	// Modo múltiplos hosts
	if multipleHosts {
		if len(args) == 0 {
			fmt.Fprintf(os.Stderr, "Erro: A opção -l requer especificar pelo menos um host\n")
			fmt.Fprintf(os.Stderr, "Uso: sc -c \"comando\" -l <host1> <host2> <host3> ...\n")
			os.Exit(1)
		}
		cmd.ConnectMultiple(cfg, args, selectedUser, useJumpHost, command)
		return
	}

	// Verifica se há argumentos (modo direto)
	if len(args) > 0 {
		hostArg := args[0]
		cmd.Connect(cfg, hostArg, selectedUser, useJumpHost, command)
		return
	}

	// Modo interativo não suporta execução de comando remoto
	if command != "" {
		fmt.Fprintf(os.Stderr, "Erro: A opção -c requer especificar um host\n")
		fmt.Fprintf(os.Stderr, "Uso: sc -c \"comando\" <host>\n")
		os.Exit(1)
	}

	// Modo interativo (menu)
	cmd.ShowInteractive(cfg, selectedUser, useJumpHost)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

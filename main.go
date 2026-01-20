package main

import (
	"fmt"
	"os"
	"time"

	"github.com/alexeiev/sshControl/cmd"
	"github.com/alexeiev/sshControl/config"
	"github.com/alexeiev/sshControl/updater"
	"github.com/spf13/cobra"
)

var (
	// Informações de versão (injetadas durante o build via ldflags)
	version   = "dev"
	buildDate = "unknown"
	gitCommit = "unknown"

	// Flags do CLI
	username      string
	jumpHost      string
	command       string
	multipleHosts bool
	showServers   bool
	showVersion   bool
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

  # Conexão direta
  sc webserver
  sc 192.168.1.50
  sc ubuntu@192.168.1.50:2222

  # Usando jump host (por nome ou índice)
  sc -j production-jump webserver
  sc -j 1 webserver
  sc -j staging-jump 192.168.1.50

  # Executar comando remoto em host único
  sc -c "uptime" webserver
  sc -u deploy -c "systemctl status nginx" webserver
  sc -j production-jump -c "cat /var/log/app.log" production-app

  # Executar comando em múltiplos hosts
  sc -c "uptime" -l web1 web2 web3
  sc -c "free -h" -l 192.168.1.10 192.168.1.11
  sc -j 1 -c "df -h" -l db1 db2 db3

  # Listar jump hosts e servidores cadastrados
  sc -s`,
	Run: runCommand,
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Atualiza o sshControl para a versão mais recente",
	Long: `Verifica se há uma nova versão disponível no GitHub e
atualiza automaticamente o binário para a versão mais recente.`,
	Example: `  # Verifica e atualiza para a versão mais recente
  sc update`,
	Run: runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)
	rootCmd.Flags().StringVarP(&username, "user", "u", "", "Nome do usuário da configuração a ser usado")
	rootCmd.Flags().StringVarP(&jumpHost, "jump", "j", "", "Jump host a usar (nome ou índice, ex: production-jump ou 1)")
	rootCmd.Flags().StringVarP(&command, "command", "c", "", "Comando a ser executado remotamente")
	rootCmd.Flags().BoolVarP(&multipleHosts, "list", "l", false, "Executa comando em múltiplos hosts (requer -c)")
	rootCmd.Flags().BoolVarP(&showServers, "servers", "s", false, "Lista jump hosts e servidores cadastrados no config")
	rootCmd.Flags().BoolVarP(&showVersion, "version", "v", false, "Exibe a versão do sshControl")
}

func runCommand(cobraCmd *cobra.Command, args []string) {
	// Verifica atualizações em background (não bloqueante, com timeout de 2s)
	checkForUpdatesBackground(version)

	// Se a flag -v foi usada, exibe a versão e sai
	if showVersion {
		fmt.Printf("sshControl (sc) versão %s\n", version)
		fmt.Printf("Build: %s\n", buildDate)
		fmt.Printf("Commit: %s\n", gitCommit)
		return
	}

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

	// Resolve o Jump Host se solicitado
	var selectedJumpHost *config.JumpHost
	if jumpHost != "" {
		if len(cfg.Config.JumpHosts) == 0 {
			fmt.Fprintf(os.Stderr, "Erro: Nenhum jump host configurado no config.yaml\n")
			os.Exit(1)
		}

		selectedJumpHost = cfg.ResolveJumpHost(jumpHost)
		if selectedJumpHost == nil {
			fmt.Fprintf(os.Stderr, "Erro: Jump host '%s' não encontrado\n", jumpHost)
			if len(cfg.Config.JumpHosts) > 0 {
				fmt.Fprintf(os.Stderr, "Jump hosts disponíveis:\n")
				for i, jh := range cfg.Config.JumpHosts {
					fmt.Fprintf(os.Stderr, "  %d. %s (%s@%s:%d)\n", i+1, jh.Name, jh.User, jh.Host, jh.Port)
				}
			}
			os.Exit(1)
		}
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
		cmd.ConnectMultiple(cfg, args, selectedUser, selectedJumpHost, command)
		return
	}

	// Verifica se há argumentos (modo direto)
	if len(args) > 0 {
		hostArg := args[0]
		cmd.Connect(cfg, hostArg, selectedUser, selectedJumpHost, command)
		return
	}

	// Modo interativo não suporta execução de comando remoto
	if command != "" {
		fmt.Fprintf(os.Stderr, "Erro: A opção -c requer especificar um host\n")
		fmt.Fprintf(os.Stderr, "Uso: sc -c \"comando\" <host>\n")
		os.Exit(1)
	}

	// Modo interativo (menu)
	cmd.ShowInteractive(cfg, selectedUser, selectedJumpHost, version)
}

func runUpdate(cobraCmd *cobra.Command, args []string) {
	fmt.Println()
	fmt.Println("🔍 Verificando atualizações...")
	fmt.Printf("Versão atual: %s\n", version)
	fmt.Println()

	u := updater.New(version)

	release, hasUpdate, err := u.CheckForUpdates()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Erro ao verificar atualizações: %v\n", err)
		os.Exit(1)
	}

	if !hasUpdate {
		fmt.Println("✅ Você já está usando a versão mais recente!")
		return
	}

	fmt.Printf("📦 Nova versão disponível: %s\n", release.TagName)
	fmt.Println()
	fmt.Print("Deseja atualizar agora? [s/N]: ")

	var response string
	fmt.Scanln(&response)

	if response != "s" && response != "S" {
		fmt.Println("Atualização cancelada.")
		return
	}

	fmt.Println()
	fmt.Println("🚀 Iniciando atualização...")

	if err := u.Update(release); err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ Erro ao atualizar: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("Execute 'sc --version' para confirmar a nova versão.")
}

// checkForUpdatesBackground verifica atualizações em background e notifica o usuário
func checkForUpdatesBackground(currentVersion string) {
	// Timeout de 2 segundos para não atrasar a execução
	done := make(chan bool, 1)

	go func() {
		u := updater.New(currentVersion)
		release, hasUpdate, err := u.CheckForUpdates()

		// Ignora erros silenciosamente (network issues, etc)
		if err != nil {
			done <- true
			return
		}

		// Se houver atualização, mostra notificação
		if hasUpdate {
			fmt.Fprintf(os.Stderr, "\n")
			fmt.Fprintf(os.Stderr, "┌─────────────────────────────────────────────────────────────┐\n")
			fmt.Fprintf(os.Stderr, "│  🔔 Nova versão disponível: %-30s  │\n", release.TagName)
			fmt.Fprintf(os.Stderr, "│  Versão atual: %-44s │\n", currentVersion)
			fmt.Fprintf(os.Stderr, "│                                                             │\n")
			fmt.Fprintf(os.Stderr, "│  Para atualizar, execute:                                   │\n")
			fmt.Fprintf(os.Stderr, "│    sc update                                                │\n")
			fmt.Fprintf(os.Stderr, "│    (ou 'sudo sc update' se necessário)                      │\n")
			fmt.Fprintf(os.Stderr, "└─────────────────────────────────────────────────────────────┘\n")
			fmt.Fprintf(os.Stderr, "\n")
		}

		done <- true
	}()

	// Aguarda até 2 segundos
	select {
	case <-done:
		return
	case <-time.After(2 * time.Second):
		return
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

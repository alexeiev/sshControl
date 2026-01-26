package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
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
	proxyEnabled  bool
	askPassword   bool
)

var rootCmd = &cobra.Command{
	Use:   "sc [flags] [host]",
	Short: "sshControl - Gerenciador de conexões SSH",
	Long: `sshControl (sc) é um gerenciador de conexões SSH que oferece modos
interativo (TUI) e CLI direto para gerenciar conexões SSH.

Suporta conexões através de jump hosts, execução de comandos remotos,
gerenciamento de múltiplos hosts em paralelo e organização por tags.

Para ver exemplos de uso e manual completo, execute: sc man`,
	Example: `  sc                           # Abre menu interativo (TUI)
  sc <host>                    # Conecta diretamente ao host
  sc -c "comando" <host>       # Executa comando remoto
  sc -c "comando" -l <hosts>   # Executa em múltiplos hosts
  sc -s                        # Lista servidores cadastrados
  sc man                       # Exibe manual completo com exemplos`,
	Args: cobra.ArbitraryArgs,
	Run:  runCommand,
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

var manCmd = &cobra.Command{
	Use:   "man",
	Short: "Exibe o manual completo do sshControl",
	Long:  "Exibe o manual completo com exemplos de uso detalhados.",
	Run:   runMan,
}

// showWithPager exibe o conteúdo usando um paginador (less, more) ou saída direta
func showWithPager(content string) {
	// Tenta usar less primeiro (melhor experiência)
	if pagerPath, err := exec.LookPath("less"); err == nil {
		pagerCmd := exec.Command(pagerPath, "-R") // -R para suportar cores/formatação
		pagerCmd.Stdin = strings.NewReader(content)
		pagerCmd.Stdout = os.Stdout
		pagerCmd.Stderr = os.Stderr
		if err := pagerCmd.Run(); err == nil {
			return
		}
	}

	// Fallback para more
	if pagerPath, err := exec.LookPath("more"); err == nil {
		pagerCmd := exec.Command(pagerPath)
		pagerCmd.Stdin = strings.NewReader(content)
		pagerCmd.Stdout = os.Stdout
		pagerCmd.Stderr = os.Stderr
		if err := pagerCmd.Run(); err == nil {
			return
		}
	}

	// Fallback final: saída direta
	fmt.Print(content)
}

func runMan(cobraCmd *cobra.Command, args []string) {
	manual := `
╔══════════════════════════════════════════════════════════════════════════════╗
║                        sshControl (sc) - Manual de Uso                       ║
╚══════════════════════════════════════════════════════════════════════════════╝

DESCRIÇÃO
  sshControl (sc) é um gerenciador de conexões SSH que oferece modo interativo
  (TUI) e CLI direto para gerenciar conexões SSH de forma eficiente.

AUTOR
  Alexeiev Araújo
  @alexeiev

CONFIGURAÇÃO
  O arquivo de configuração fica em: ~/.sshControl/config.yaml
  Na primeira execução, um template é criado automaticamente.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

MODO INTERATIVO (TUI)
  sc                        Abre menu interativo para selecionar host
  sc -u <usuario>           Menu com usuário específico
  sc -j <jump>              Menu via jump host
  sc -p                     Menu com proxy reverso habilitado

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

CONEXÃO DIRETA
  sc <host>                        Conecta a host do config.yaml
  sc 192.168.1.50                  Conecta diretamente a IP
  sc ubuntu@192.168.1.50           Especifica usuário
  sc ubuntu@192.168.1.50:2222      Especifica usuário e porta
  sc -j production-jump <host>     Conecta via jump host (por nome)
  sc -j 1 <host>                   Conecta via jump host (por índice)
  sc -p <host>                     Conecta com proxy reverso

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

EXECUÇÃO DE COMANDOS REMOTOS (Host Único)
  sc -c "uptime" <host>                   Executa comando no host
  sc -c "df -h" 192.168.1.50              Executa em IP direto
  sc -u deploy -c "systemctl status nginx" <host>
                                          Com usuário específico
  sc -j 1 -c "cat /var/log/app.log" <host>
                                          Via jump host
  sc -a -c "comando" <host>               Solicita senha antes

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

EXECUÇÃO EM MÚLTIPLOS HOSTS
  sc -c "uptime" -l web1 web2 web3        Em vários hosts do config
  sc -c "free -h" -l 192.168.1.10 192.168.1.11
                                          Em múltiplos IPs
  sc -c "hostname" -l web1 192.168.1.50   Combina hosts e IPs
  sc -j 1 -c "df -h" -l db1 db2 db3       Via jump host
  sc -a -c "uptime" -l web1 web2 web3     Solicita senha uma vez antes

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

TAGS (Agrupamento de Hosts)
  Hosts podem ter tags no config.yaml para agrupamento:

  hosts:
    - name: web1
      host: 192.168.1.10
      port: 22
      tags: [web, production]

  Use @tag para executar em todos os hosts de uma tag:
  sc -c "uptime" -l @web                  Todos os hosts com tag "web"
  sc -c "df -h" -l @web @db               Múltiplas tags
  sc -c "hostname" -l @production server1 Combina tag e host

  Na TUI, digite "/" e busque pelo nome da tag para filtrar.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

AUTO-CRIAÇÃO DE HOSTS
  Com auto_create: true no config.yaml, hosts não cadastrados são salvos
  automaticamente após conexão bem-sucedida com a tag "autocreated".

  Hosts com tag "autocreated" não aparecem na TUI, mas podem ser usados:
  sc -c "uptime" -l @autocreated          Executa nos hosts auto-criados
  sc -s                                   Lista inclui hosts autocreated

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

PROXY REVERSO
  Compartilha proxy HTTP/HTTPS/FTP da máquina local com hosts remotos.
  Configure no config.yaml:
    config:
      proxy: "192.168.0.1:3128"
      proxy_port: 9999

  sc -p <host>                            Conecta com proxy habilitado

  No host remoto, configure:
  export {https,http,ftp}_proxy=http://127.0.0.1:9999

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

COMANDOS ÚTEIS
  sc -s                     Lista servidores e jump hosts cadastrados
  sc -v, sc --version       Exibe versão do sshControl
  sc update                 Atualiza para versão mais recente
  sc man                    Exibe este manual
  sc --help                 Exibe ajuda rápida

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

FLAGS DISPONÍVEIS
  -u, --user <usuario>      Usuário SSH a ser usado
  -j, --jump <jump>         Jump host (nome ou índice)
  -c, --command <comando>   Comando a executar remotamente
  -l, --list                Modo múltiplos hosts (requer -c)
  -s, --servers             Lista servidores cadastrados
  -p, --proxy               Habilita proxy reverso
  -a, --ask-password        Solicita senha antes de conectar
  -v, --version             Exibe versão
  -h, --help                Exibe ajuda

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

AUTENTICAÇÃO
  Ordem de tentativa:
  1. Chave SSH (configurada no config.yaml)
  2. SSH Agent (se disponível)
  3. Senha (interativa ou via -a)

  A flag -a solicita senha antes de tentar conectar, útil para:
  - Primeira conexão (antes de instalar chave)
  - Automações em múltiplos hosts
  - Servidores sem chave configurada

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

MAIS INFORMAÇÕES
  Repositório: https://github.com/alexeiev/sshControl
  Issues:      https://github.com/alexeiev/sshControl/issues

`
	showWithPager(manual)
}

func init() {
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(manCmd)
	rootCmd.Flags().StringVarP(&username, "user", "u", "", "Nome do usuário da configuração a ser usado")
	rootCmd.Flags().StringVarP(&jumpHost, "jump", "j", "", "Jump host a usar (nome ou índice, ex: production-jump ou 1)")
	rootCmd.Flags().StringVarP(&command, "command", "c", "", "Comando a ser executado remotamente")
	rootCmd.Flags().BoolVarP(&multipleHosts, "list", "l", false, "Executa comando em múltiplos hosts (requer -c)")
	rootCmd.Flags().BoolVarP(&showServers, "servers", "s", false, "Lista jump hosts e servidores cadastrados no config")
	rootCmd.Flags().BoolVarP(&showVersion, "version", "v", false, "Exibe a versão do sshControl")
	rootCmd.Flags().BoolVarP(&proxyEnabled, "proxy", "p", false, "Habilita tunnel SSH reverso para compartilhar proxy")
	rootCmd.Flags().BoolVarP(&askPassword, "ask-password", "a", false, "Solicita senha antes de tentar autenticação (útil para automações)")
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
		cmd.ConnectMultiple(cfg, configPath, args, selectedUser, selectedJumpHost, command, proxyEnabled, askPassword)
		return
	}

	// Verifica se há argumentos (modo direto)
	if len(args) > 0 {
		hostArg := args[0]
		cmd.Connect(cfg, configPath, hostArg, selectedUser, selectedJumpHost, command, proxyEnabled, askPassword)
		return
	}

	// Modo interativo não suporta execução de comando remoto
	if command != "" {
		fmt.Fprintf(os.Stderr, "Erro: A opção -c requer especificar um host\n")
		fmt.Fprintf(os.Stderr, "Uso: sc -c \"comando\" <host>\n")
		os.Exit(1)
	}

	// Modo interativo (menu)
	cmd.ShowInteractive(cfg, selectedUser, selectedJumpHost, version, proxyEnabled)
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

	// Exibe as release notes se disponíveis
	if release.Body != "" {
		fmt.Println("📝 O que há de novo:")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println(release.Body)
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println()
	}

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
			fmt.Fprintf(os.Stderr, "│  Para atualizar e ver as novidades, execute:                │\n")
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

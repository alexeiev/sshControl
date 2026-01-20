package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alexeiev/sshControl/config"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Estilos
var (
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4")).
			Bold(true).
			Padding(0, 1)

	bannerStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4")).
			Padding(0, 2).
			MarginBottom(1)

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262"))

	userInfoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575")).
			Bold(true)

	jumpHostEnabledStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#04B575")).
				Bold(true)

	jumpHostDisabledStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF6B6B"))

	connectionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575")).
			Bold(true).
			Padding(1, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#04B575"))
)

// hostItem implementa list.Item para trabalhar com bubbles/list
type hostItem struct {
	host          config.Host
	sshKey        string
	effectiveUser string
}

func (i hostItem) FilterValue() string {
	return i.host.Name + " " + i.host.Host
}

func (i hostItem) Title() string {
	return i.host.Name
}

func (i hostItem) Description() string {
	return i.host.Host
}

// model representa o estado da aplicação
type model struct {
	list           list.Model
	filter         textinput.Model
	filterActive   bool
	cfg            *config.ConfigFile
	selectedUser   *config.User
	jumpHost       *config.JumpHost
	allItems       []list.Item
	selectedHost   *config.Host
	selectedSSHKey string
	effectiveUser  string
	quitting       bool
}

// ShowInteractive exibe o menu interativo usando bubbletea
func ShowInteractive(cfg *config.ConfigFile, selectedUser *config.User, jumpHost *config.JumpHost) {
	if len(cfg.Hosts) == 0 {
		fmt.Println("Nenhum host configurado no arquivo config.yaml")
		return
	}

	// Cria os items da lista
	items := make([]list.Item, len(cfg.Hosts))

	// Determina o usuário efetivo para esta sessão
	effectiveUser := cfg.GetEffectiveUser(selectedUser)
	if effectiveUser == nil {
		fmt.Println("Erro: Nenhum usuário configurado")
		return
	}

	// Obtém a chave SSH do usuário efetivo
	sshKey := ""
	if len(effectiveUser.SSHKeys) > 0 {
		sshKey = config.ExpandHomePath(effectiveUser.SSHKeys[0])
	}

	for i, h := range cfg.Hosts {
		items[i] = hostItem{
			host:          h,
			sshKey:        sshKey,
			effectiveUser: effectiveUser.Name,
		}
	}

	// Cria o filtro de texto
	ti := textinput.New()
	ti.Placeholder = "Filtrar hosts..."
	ti.CharLimit = 50

	// Cria a lista
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "SSH Control - Selecione um host"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)

	m := model{
		list:         l,
		filter:       ti,
		filterActive: false,
		cfg:          cfg,
		selectedUser: selectedUser,
		jumpHost:     jumpHost,
		allItems:     items,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erro ao executar o menu: %v\n", err)
		os.Exit(1)
	}

	// Conecta ao host selecionado
	if m, ok := finalModel.(model); ok && m.selectedHost != nil {
		// Busca a chave SSH do jump host se estiver usando jump host
		jumpHostSSHKey := ""
		if m.jumpHost != nil {
			jumpHostSSHKey = m.cfg.GetJumpHostSSHKey(m.jumpHost)
		}

		sshConn := NewSSHConnection(
			m.effectiveUser,
			m.selectedHost.Host,
			m.selectedHost.Port,
			m.selectedSSHKey,
			"", // Senha vazia - será pedida interativamente se necessário
			m.jumpHost,
			jumpHostSSHKey,
			"", // Modo interativo não executa comandos remotos
		)

		if err := sshConn.Connect(); err != nil {
			fmt.Fprintf(os.Stderr, "\n❌ Erro na conexão SSH: %v\n", err)
			os.Exit(1)
		}
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Se está filtrando
		if m.filterActive {
			switch msg.String() {
			case "esc":
				m.filterActive = false
				m.filter.SetValue("")
				m.list.SetItems(m.allItems)
				return m, nil
			case "enter":
				m.filterActive = false
				return m, nil
			default:
				var cmd tea.Cmd
				m.filter, cmd = m.filter.Update(msg)
				m.applyFilter()
				return m, cmd
			}
		}

		// Navegação normal
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "/":
			m.filterActive = true
			m.filter.Focus()
			return m, nil

		case "enter":
			if i, ok := m.list.SelectedItem().(hostItem); ok {
				m.selectedHost = &i.host
				m.selectedSSHKey = i.sshKey
				m.effectiveUser = i.effectiveUser
				return m, tea.Quit
			}
		}

	case tea.WindowSizeMsg:
		h, v := lipgloss.NewStyle().GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v-8)
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	now := time.Now()

	// Informação do usuário SSH
	sshUserInfo := infoStyle.Render("not configured")
	if m.selectedUser != nil {
		sshUserInfo = userInfoStyle.Render(m.selectedUser.Name)
	} else if defaultUser := m.cfg.GetDefaultUser(); defaultUser != nil {
		sshUserInfo = userInfoStyle.Render(defaultUser.Name)
	}

	// Status do Jump Host
	jumpHostStatus := jumpHostDisabledStyle.Render("None")
	if m.jumpHost != nil {
		jumpHostStatus = jumpHostEnabledStyle.Render(m.jumpHost.Name)
	}

	banner := fmt.Sprintf(
		"%s  |  SSH User: %s  |  Jump Host: %s  |  %s",
		titleStyle.Render("🚀 SSH Control"),
		sshUserInfo,
		jumpHostStatus,
		now.Format("02/01/2006 15:04:05"),
	)

	// Filtro
	filterView := ""
	if m.filterActive {
		filterView = "\n" + m.filter.View() + "\n"
	} else {
		filterView = "\n" + infoStyle.Render("Pressione '/' para filtrar, 'Enter' para conectar, 'q' para sair") + "\n"
	}

	return bannerStyle.Render(banner) + filterView + "\n" + m.list.View()
}

// applyFilter filtra os items baseado no texto digitado
func (m *model) applyFilter() {
	filterText := strings.ToLower(m.filter.Value())
	if filterText == "" {
		m.list.SetItems(m.allItems)
		return
	}

	var filtered []list.Item
	for _, item := range m.allItems {
		if strings.Contains(strings.ToLower(item.FilterValue()), filterText) {
			filtered = append(filtered, item)
		}
	}

	m.list.SetItems(filtered)
}

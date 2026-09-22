package cmd

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/alexeiev/sshControl/config"
)

// TunnelRoutes representa as rotas estáticas de um tunnel (gateway + redes)
type TunnelRoutes struct {
	Name     string   `json:"name,omitempty"` // Jump host do tunnel (preenchido ao gravar o estado)
	Gateway  string   `json:"gateway"`
	Networks []string `json:"networks"`
}

// routeRunner executa um comando de rota e retorna a saída combinada.
// É uma variável para permitir substituição nos testes.
var routeRunner = runRouteCommand

// ParseTunnelRoutes valida e normaliza as rotas configuradas no jump host.
// Retorna nil quando gateway ou networks não estão preenchidos.
func ParseTunnelRoutes(jh *config.JumpHost) (*TunnelRoutes, error) {
	if jh.Routes == nil || (jh.Routes.Gateway == "" && len(jh.Routes.Networks) == 0) {
		return nil, nil
	}
	if !jh.HasRoutes() {
		fmt.Fprintf(os.Stderr, "⚠️  Aviso: rotas do jump host '%s' ignoradas (preencha gateway e networks)\n", jh.Name)
		return nil, nil
	}

	gateway := net.ParseIP(strings.TrimSpace(jh.Routes.Gateway))
	if gateway == nil {
		return nil, fmt.Errorf("gateway inválido nas rotas do jump host '%s': %q", jh.Name, jh.Routes.Gateway)
	}

	routes := &TunnelRoutes{Gateway: gateway.String()}
	seen := make(map[string]bool)
	for _, network := range jh.Routes.Networks {
		cidr, err := normalizeNetwork(network)
		if err != nil {
			return nil, fmt.Errorf("rede inválida nas rotas do jump host '%s': %w", jh.Name, err)
		}
		if !seen[cidr] {
			seen[cidr] = true
			routes.Networks = append(routes.Networks, cidr)
		}
	}

	// Aviso se o jump host (quando informado por IP) não estiver coberto pelas rotas
	if ip := net.ParseIP(jh.Host); ip != nil && !routes.covers(ip) {
		fmt.Fprintf(os.Stderr, "⚠️  Aviso: o jump host %s não está em nenhuma rede de routes.networks\n", jh.Host)
	}

	return routes, nil
}

// normalizeNetwork converte a rede para CIDR canônico (um IP sem máscara vira /32 ou /128)
func normalizeNetwork(network string) (string, error) {
	network = strings.TrimSpace(network)
	if !strings.Contains(network, "/") {
		ip := net.ParseIP(network)
		if ip == nil {
			return "", fmt.Errorf("%q", network)
		}
		if ip.To4() != nil {
			return ip.String() + "/32", nil
		}
		return ip.String() + "/128", nil
	}

	_, ipNet, err := net.ParseCIDR(network)
	if err != nil {
		return "", fmt.Errorf("%q", network)
	}
	return ipNet.String(), nil
}

// covers verifica se o IP pertence a alguma das redes
func (r *TunnelRoutes) covers(ip net.IP) bool {
	for _, network := range r.Networks {
		if _, ipNet, err := net.ParseCIDR(network); err == nil && ipNet.Contains(ip) {
			return true
		}
	}
	return false
}

// routeCommandArgs monta o comando de rota para o sistema operacional
func routeCommandArgs(goos, action, network, gateway string) ([]string, error) {
	switch goos {
	case "darwin":
		verb := "add"
		if action == "del" {
			verb = "delete"
		}
		args := []string{"route", "-n", verb}
		if strings.Contains(network, ":") {
			args = append(args, "-inet6")
		}
		return append(args, "-net", network, gateway), nil
	case "linux":
		return []string{"ip", "route", action, network, "via", gateway}, nil
	default:
		return nil, fmt.Errorf("criação de rotas não suportada em %s", goos)
	}
}

// runRouteCommand executa o comando, usando sudo quando não for root.
// Com interactive=false usa 'sudo -n' (falha em vez de solicitar senha).
func runRouteCommand(args []string, interactive bool) (string, error) {
	if os.Geteuid() != 0 {
		sudoArgs := []string{"sudo"}
		if !interactive {
			sudoArgs = append(sudoArgs, "-n")
		}
		args = append(sudoArgs, args...)
	}

	command := exec.Command(args[0], args[1:]...)
	if interactive {
		command.Stdin = os.Stdin
	}
	out, err := command.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// routeAlreadyExists identifica o erro de rota já existente (macOS e Linux)
func routeAlreadyExists(output string) bool {
	return strings.Contains(output, "File exists")
}

// routeNotFound identifica o erro de rota inexistente na remoção (macOS e Linux)
func routeNotFound(output string) bool {
	return strings.Contains(output, "not in table") || strings.Contains(output, "No such process")
}

// AddRoutes cria as rotas e retorna apenas as redes efetivamente adicionadas.
// Rotas que já existiam são mantidas e não entram no retorno (não serão removidas depois).
// Em caso de erro, desfaz as rotas adicionadas nesta chamada.
func (r *TunnelRoutes) AddRoutes(interactive bool) (*TunnelRoutes, error) {
	added := &TunnelRoutes{Gateway: r.Gateway}

	for _, network := range r.Networks {
		args, err := routeCommandArgs(runtime.GOOS, "add", network, r.Gateway)
		if err != nil {
			return nil, err
		}

		out, err := routeRunner(args, interactive)
		if err != nil {
			if routeAlreadyExists(out) {
				fmt.Printf("   ℹ️  Rota %s já existe, será mantida\n", network)
				continue
			}
			added.RemoveRoutes(interactive)
			return nil, fmt.Errorf("erro ao adicionar rota %s via %s: %v %s", network, r.Gateway, err, out)
		}

		fmt.Printf("   ✅ Rota %s via %s adicionada\n", network, r.Gateway)
		added.Networks = append(added.Networks, network)
	}

	return added, nil
}

// RemoveRoutes remove as rotas e retorna as redes que não puderam ser removidas
func (r *TunnelRoutes) RemoveRoutes(interactive bool) []string {
	var failed []string

	for _, network := range r.Networks {
		args, err := routeCommandArgs(runtime.GOOS, "del", network, r.Gateway)
		if err != nil {
			failed = append(failed, network)
			continue
		}

		out, err := routeRunner(args, interactive)
		if err != nil && !routeNotFound(out) {
			if interactive {
				fmt.Fprintf(os.Stderr, "   ⚠️  Falha ao remover rota %s: %v %s\n", network, err, out)
			}
			failed = append(failed, network)
			continue
		}

		if interactive {
			fmt.Printf("   🧹 Rota %s via %s removida\n", network, r.Gateway)
		}
	}

	return failed
}

func tunnelRoutesPath(stateDir, name string) string {
	return tunnelFileBase(stateDir, name) + ".routes"
}

// saveTunnelRoutes grava as rotas criadas pelo tunnel (remove o arquivo se não houver rotas)
func saveTunnelRoutes(stateDir, name string, routes *TunnelRoutes) error {
	path := tunnelRoutesPath(stateDir, name)
	if routes == nil || len(routes.Networks) == 0 {
		os.Remove(path)
		return nil
	}

	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return fmt.Errorf("erro ao criar diretório %s: %w", stateDir, err)
	}
	data, err := json.Marshal(TunnelRoutes{Name: name, Gateway: routes.Gateway, Networks: routes.Networks})
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("erro ao gravar rotas em %s: %w", path, err)
	}
	return nil
}

// loadTunnelRoutes lê as rotas criadas por um tunnel (nil se não houver)
func loadTunnelRoutes(stateDir, name string) *TunnelRoutes {
	data, err := os.ReadFile(tunnelRoutesPath(stateDir, name))
	if err != nil {
		return nil
	}
	var routes TunnelRoutes
	if err := json.Unmarshal(data, &routes); err != nil || len(routes.Networks) == 0 {
		return nil
	}
	return &routes
}

// cleanupTunnelRoutes remove as rotas registradas de um tunnel.
// As redes que não puderem ser removidas continuam registradas para nova tentativa.
func cleanupTunnelRoutes(stateDir, name string, interactive bool) error {
	routes := loadTunnelRoutes(stateDir, name)
	if routes == nil {
		return nil
	}

	if interactive {
		fmt.Printf("🛣️  Removendo rotas do tunnel via %s...\n", name)
	}
	failed := routes.RemoveRoutes(interactive)
	saveTunnelRoutes(stateDir, name, &TunnelRoutes{Gateway: routes.Gateway, Networks: failed})

	if len(failed) > 0 {
		return fmt.Errorf("rotas não removidas: %s (execute 'sc tunnel stop -j %s')", strings.Join(failed, ", "), name)
	}
	return nil
}

// pendingTunnelRoutes lista as rotas registradas de tunnels que não estão mais em execução.
// Se name não for vazio, considera apenas o tunnel desse jump host.
func pendingTunnelRoutes(stateDir, name string, running []TunnelInfo) []*TunnelRoutes {
	active := make(map[string]bool)
	for _, tn := range running {
		active[tn.Name] = true
	}

	paths, _ := filepath.Glob(filepath.Join(stateDir, "*.routes"))
	sort.Strings(paths)

	var pending []*TunnelRoutes
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var routes TunnelRoutes
		if err := json.Unmarshal(data, &routes); err != nil || len(routes.Networks) == 0 || routes.Name == "" {
			continue
		}
		if active[routes.Name] || (name != "" && routes.Name != name) {
			continue
		}
		pending = append(pending, &routes)
	}
	return pending
}

// setupTunnelRoutes remove rotas pendentes de uma execução anterior e cria as rotas do tunnel
func setupTunnelRoutes(stateDir, name string, routes *TunnelRoutes) error {
	if err := cleanupTunnelRoutes(stateDir, name, true); err != nil {
		return err
	}
	if routes == nil {
		return nil
	}

	fmt.Printf("🛣️  Adicionando rotas para alcançar %s (pode ser solicitada a senha do sudo)...\n", name)
	added, err := routes.AddRoutes(true)
	if err != nil {
		return err
	}
	if err := saveTunnelRoutes(stateDir, name, added); err != nil {
		added.RemoveRoutes(true)
		return err
	}
	return nil
}

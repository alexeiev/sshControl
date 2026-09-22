package cmd

import (
	"errors"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/alexeiev/sshControl/config"
)

// fakeRouteRunner simula a tabela de rotas do sistema sem executar comandos reais
type fakeRouteRunner struct {
	table    map[string]bool // redes presentes na tabela
	failAdd  map[string]bool // redes cuja adição deve falhar
	failDel  bool            // remoção falha (ex: sudo -n sem credenciais)
	commands [][]string
}

func newFakeRouteRunner(t *testing.T) *fakeRouteRunner {
	t.Helper()
	f := &fakeRouteRunner{table: map[string]bool{}, failAdd: map[string]bool{}}
	original := routeRunner
	routeRunner = f.run
	t.Cleanup(func() { routeRunner = original })
	return f
}

func (f *fakeRouteRunner) run(args []string, interactive bool) (string, error) {
	f.commands = append(f.commands, args)

	// Localiza a ação e a rede no comando (macOS: route -n add|delete ... -net <rede> <gw>; Linux: ip route add|del <rede> via <gw>)
	var action, network string
	for i, arg := range args {
		switch arg {
		case "add":
			action = "add"
		case "delete", "del":
			action = "del"
		case "-net":
			network = args[i+1]
		}
	}
	if network == "" && len(args) > 3 && args[0] == "ip" {
		network = args[3]
	}

	switch action {
	case "add":
		if f.failAdd[network] {
			return "Network is unreachable", errors.New("exit status 1")
		}
		if f.table[network] {
			return "File exists", errors.New("exit status 1")
		}
		f.table[network] = true
	case "del":
		if f.failDel {
			return "sudo: a password is required", errors.New("exit status 1")
		}
		if !f.table[network] {
			return "not in table", errors.New("exit status 1")
		}
		delete(f.table, network)
	}
	return "", nil
}

func TestParseTunnelRoutes(t *testing.T) {
	tests := []struct {
		name    string
		routes  *config.JumpHostRoutes
		want    *TunnelRoutes
		wantErr bool
	}{
		{name: "sem rotas", routes: nil, want: nil},
		{name: "apenas gateway é ignorado", routes: &config.JumpHostRoutes{Gateway: "192.168.1.36"}, want: nil},
		{name: "apenas networks é ignorado", routes: &config.JumpHostRoutes{Networks: []string{"10.0.0.0/8"}}, want: nil},
		{
			name:   "normaliza e remove duplicadas",
			routes: &config.JumpHostRoutes{Gateway: " 192.168.1.36 ", Networks: []string{"10.1.2.3/8", "10.0.0.0/8", "172.16.0.5"}},
			want:   &TunnelRoutes{Gateway: "192.168.1.36", Networks: []string{"10.0.0.0/8", "172.16.0.5/32"}},
		},
		{name: "gateway inválido", routes: &config.JumpHostRoutes{Gateway: "gw.local", Networks: []string{"10.0.0.0/8"}}, wantErr: true},
		{name: "rede inválida", routes: &config.JumpHostRoutes{Gateway: "192.168.1.36", Networks: []string{"10.0.0.0/33"}}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jh := &config.JumpHost{Name: "main-jump", Host: "10.177.165.25", Routes: tt.routes}
			got, err := ParseTunnelRoutes(jh)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseTunnelRoutes erro = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ParseTunnelRoutes = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRouteCommandArgs(t *testing.T) {
	tests := []struct {
		goos, action, network string
		want                  []string
	}{
		{"darwin", "add", "10.0.0.0/8", []string{"route", "-n", "add", "-net", "10.0.0.0/8", "192.168.1.36"}},
		{"darwin", "del", "10.0.0.0/8", []string{"route", "-n", "delete", "-net", "10.0.0.0/8", "192.168.1.36"}},
		{"linux", "add", "10.0.0.0/8", []string{"ip", "route", "add", "10.0.0.0/8", "via", "192.168.1.36"}},
		{"linux", "del", "10.0.0.0/8", []string{"ip", "route", "del", "10.0.0.0/8", "via", "192.168.1.36"}},
	}
	for _, tt := range tests {
		got, err := routeCommandArgs(tt.goos, tt.action, tt.network, "192.168.1.36")
		if err != nil || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("routeCommandArgs(%s, %s) = %v, %v; want %v", tt.goos, tt.action, got, err, tt.want)
		}
	}

	if _, err := routeCommandArgs("windows", "add", "10.0.0.0/8", "192.168.1.36"); err == nil {
		t.Errorf("routeCommandArgs(windows) deveria retornar erro")
	}
}

func skipIfRoutesUnsupported(t *testing.T) {
	t.Helper()
	if _, err := routeCommandArgs(runtime.GOOS, "add", "10.0.0.0/8", "192.168.1.1"); err != nil {
		t.Skipf("rotas não suportadas em %s", runtime.GOOS)
	}
}

func TestAddRoutesKeepsExistingAndRollsBackOnError(t *testing.T) {
	skipIfRoutesUnsupported(t)
	fake := newFakeRouteRunner(t)

	// Rota pré-existente não é registrada como criada pelo tunnel
	fake.table["172.16.0.0/12"] = true
	routes := &TunnelRoutes{Gateway: "192.168.1.36", Networks: []string{"10.0.0.0/8", "172.16.0.0/12"}}
	added, err := routes.AddRoutes(true)
	if err != nil {
		t.Fatalf("AddRoutes retornou erro: %v", err)
	}
	if !reflect.DeepEqual(added.Networks, []string{"10.0.0.0/8"}) {
		t.Fatalf("redes adicionadas = %v, want [10.0.0.0/8]", added.Networks)
	}

	// Falha na segunda rota desfaz a primeira
	fake2 := newFakeRouteRunner(t)
	fake2.failAdd["192.168.50.0/24"] = true
	routes = &TunnelRoutes{Gateway: "192.168.1.36", Networks: []string{"10.0.0.0/8", "192.168.50.0/24"}}
	if _, err := routes.AddRoutes(true); err == nil {
		t.Fatalf("AddRoutes deveria retornar erro")
	}
	if len(fake2.table) != 0 {
		t.Fatalf("rotas não foram desfeitas após erro: %v", fake2.table)
	}
}

func TestTunnelRoutesLifecycle(t *testing.T) {
	skipIfRoutesUnsupported(t)
	fake := newFakeRouteRunner(t)
	stateDir := t.TempDir()

	routes := &TunnelRoutes{Gateway: "192.168.1.36", Networks: []string{"10.0.0.0/8"}}
	if err := setupTunnelRoutes(stateDir, "main-jump", routes); err != nil {
		t.Fatalf("setupTunnelRoutes retornou erro: %v", err)
	}
	if !fake.table["10.0.0.0/8"] {
		t.Fatalf("rota não foi criada")
	}

	// Remoção sem credenciais (daemon com sudo -n) mantém o registro para nova tentativa
	fake.failDel = true
	if err := cleanupTunnelRoutes(stateDir, "main-jump", false); err == nil {
		t.Fatalf("cleanupTunnelRoutes deveria falhar sem credenciais")
	}
	pending := pendingTunnelRoutes(stateDir, "", nil)
	if len(pending) != 1 || pending[0].Name != "main-jump" {
		t.Fatalf("pendingTunnelRoutes = %+v, want main-jump", pending)
	}

	// 'sc tunnel stop' remove as rotas pendentes mesmo sem tunnel em execução
	fake.failDel = false
	if err := StopTunnels(stateDir, "main-jump"); err != nil {
		t.Fatalf("StopTunnels retornou erro: %v", err)
	}
	if fake.table["10.0.0.0/8"] {
		t.Fatalf("rota não foi removida")
	}
	if _, err := os.Stat(tunnelRoutesPath(stateDir, "main-jump")); !os.IsNotExist(err) {
		t.Fatalf("arquivo de rotas não foi removido")
	}
	if err := StopTunnels(stateDir, "main-jump"); err == nil || !strings.Contains(err.Error(), "nenhum tunnel ativo") {
		t.Fatalf("StopTunnels sem tunnel/rotas = %v, want 'nenhum tunnel ativo'", err)
	}
}

func TestSetupTunnelRoutesCleansLeftovers(t *testing.T) {
	skipIfRoutesUnsupported(t)
	fake := newFakeRouteRunner(t)
	stateDir := t.TempDir()

	// Rota que ficou de uma execução anterior (tunnel caiu e o sudo -n falhou)
	fake.table["10.0.0.0/8"] = true
	saveTunnelRoutes(stateDir, "main-jump", &TunnelRoutes{Gateway: "192.168.1.36", Networks: []string{"10.0.0.0/8"}})

	routes := &TunnelRoutes{Gateway: "192.168.1.36", Networks: []string{"10.0.0.0/8"}}
	if err := setupTunnelRoutes(stateDir, "main-jump", routes); err != nil {
		t.Fatalf("setupTunnelRoutes retornou erro: %v", err)
	}

	// A rota antiga é removida e recriada, continuando registrada como criada pelo tunnel
	got := loadTunnelRoutes(stateDir, "main-jump")
	if got == nil || !reflect.DeepEqual(got.Networks, []string{"10.0.0.0/8"}) {
		t.Fatalf("rotas registradas = %+v, want [10.0.0.0/8]", got)
	}
	if !fake.table["10.0.0.0/8"] {
		t.Fatalf("rota não foi recriada")
	}
}

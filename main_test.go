package main

import (
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestIgnoreHostKeyFlagIsInheritedByAllSSHCommands(t *testing.T) {
	for _, command := range []*cobra.Command{rootCmd, cpDownCmd, cpUpCmd, pfCmd, tunnelCmd} {
		for _, flag := range []string{"--ignore-host-key", "-i"} {
			t.Run(command.CommandPath()+"/"+flag, func(t *testing.T) {
				ignoreHostKey = false
				t.Cleanup(func() { ignoreHostKey = false })
				if err := command.ParseFlags([]string{flag}); err != nil {
					t.Fatal(err)
				}
				if !ignoreHostKey {
					t.Fatal("flag não propagada")
				}
			})
		}
	}
}

// Executa a árvore real do Cobra, substituindo somente a operação final para
// conferir o parsing sem abrir conexões ou modificar a configuração do usuário.
func executeCLIForFlags(t *testing.T, args []string, check func(*cobra.Command, []string)) error {
	t.Helper()
	var visit func(*cobra.Command)
	visit = func(c *cobra.Command) {
		for _, child := range c.Commands() {
			visit(child)
		}
		reset := func(f *pflag.Flag) {
			value, changed := f.Value.String(), f.Changed
			t.Cleanup(func() {
				if err := f.Value.Set(value); err != nil {
					t.Error(err)
				}
				f.Changed = changed
			})
			if err := f.Value.Set(f.DefValue); err != nil {
				t.Fatal(err)
			}
			f.Changed = false
		}
		c.Flags().VisitAll(reset)
		c.PersistentFlags().VisitAll(reset)
		if c.Run != nil || c.RunE != nil {
			run, runE := c.Run, c.RunE
			t.Cleanup(func() { c.Run, c.RunE = run, runE })
			c.Run = nil
			c.RunE = func(c *cobra.Command, args []string) error {
				check(c, args)
				return nil
			}
		}
	}
	visit(rootCmd)
	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(io.Discard)
	rootCmd.SetArgs(args)
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})
	_, err := rootCmd.ExecuteC()
	return err
}

func TestSSHFlagsAcrossCommandsAndPositions(t *testing.T) {
	for _, command := range []struct {
		path []string
		args []string
	}{
		{nil, []string{"server"}},
		{[]string{"cp", "up"}, []string{"/tmp/test.up", "100.64.2.11"}},
		{[]string{"cp", "down"}, []string{"server", "/tmp/test.down"}},
		{[]string{"port-forward"}, []string{"server", "8080:80"}},
		{[]string{"tunnel"}, nil},
	} {
		wantCommand := strings.Join(append([]string{"sc"}, command.path...), " ")
		for _, flag := range []struct {
			name        string
			short, long []string
			value       func() any
			want        any
		}{
			{"user", []string{"-u1"}, []string{"--user=1"}, func() any { return username }, "1"},
			{"jump", []string{"-j2"}, []string{"--jump=2"}, func() any { return jumpHost }, "2"},
			{"ask-password", []string{"-a"}, []string{"--ask-password"}, func() any { return askPassword }, true},
			{"env-password", []string{"-P"}, []string{"--env-password"}, func() any { return envPassword }, true},
			{"verbose", []string{"-v"}, []string{"--verbose"}, func() any { return verbose }, true},
			{"ignore-host-key", []string{"-i"}, []string{"--ignore-host-key"}, func() any { return ignoreHostKey }, true},
		} {
			spellings := [][]string{flag.short, flag.long}
			if value, ok := flag.want.(string); ok {
				spellings = append(spellings, []string{flag.short[0][:2], value}, []string{"--" + flag.name, value})
			}
			for _, spelling := range spellings {
				for position := 0; position <= len(command.path)+1; position++ {
					args := append(append([]string{}, command.path...), command.args...)
					index := position
					if index > len(command.path) {
						index = len(args)
					}
					args = append(append(append([]string{}, args[:index]...), spelling...), args[index:]...)
					t.Run(strings.Join(args, " "), func(t *testing.T) {
						called := false
						err := executeCLIForFlags(t, args, func(c *cobra.Command, positional []string) {
							called = true
							if c.CommandPath() != wantCommand {
								t.Fatalf("comando inesperado: %s", c.CommandPath())
							}
							if !slices.Equal(positional, command.args) {
								t.Fatalf("argumentos = %v; esperado %v", positional, command.args)
							}
							if got := flag.value(); got != flag.want {
								t.Fatalf("%s = %v; esperado %v", flag.name, got, flag.want)
							}
						})
						if err != nil || !called {
							t.Fatalf("flag não chegou ao comando: %v", err)
						}
					})
				}
			}
		}
	}
}

func TestCommandSpecificFlagsAndCombinations(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want []any
	}{
		{[]string{"-j2", "-u1", "cp", "up", "-rPv", "/tmp/test.up", "100.64.2.11"}, []any{"1", "2", true, true, false, true, false, false, false, "", 4000}},
		{[]string{"cp", "--user", "deploy", "--jump", "prod", "up", "--recursive", "--list", "--env-password", "--verbose", "--ignore-host-key", "f", "server"}, []any{"deploy", "prod", true, true, false, true, true, true, false, "", 4000}},
		{[]string{"cp", "down", "-r", "server", "remote"}, []any{"", "", true, false, false, false, false, false, false, "", 4000}},
		{[]string{"cp", "--recursive", "up", "f", "server"}, []any{"", "", true, false, false, false, false, false, false, "", 4000}},
		{[]string{"cp", "up", "-l", "f", "server"}, []any{"", "", false, false, false, false, false, true, false, "", 4000}},
		{[]string{"-v", "cp", "up", "--verbose=false", "f", "server"}, []any{"", "", false, false, false, false, false, false, false, "", 4000}},
		{[]string{"-c", "uptime", "-lp", "server"}, []any{"", "", false, false, false, false, false, true, true, "uptime", 4000}},
		{[]string{"--command=uptime", "--list", "--proxy", "server"}, []any{"", "", false, false, false, false, false, true, true, "uptime", 4000}},
		{[]string{"-j2", "tunnel", "-u1", "--port", "5000", "-a"}, []any{"1", "2", false, false, true, false, false, false, false, "", 5000}},
		{[]string{"-j2", "tunnel", "stop"}, []any{"", "2", false, false, false, false, false, false, false, "", 4000}},
		{[]string{"tunnel", "stop", "--jump", "prod"}, []any{"", "prod", false, false, false, false, false, false, false, "", 4000}},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			called := false
			err := executeCLIForFlags(t, tc.args, func(c *cobra.Command, _ []string) {
				called = true
				got := []any{username, jumpHost, cpRecursive, envPassword, askPassword, verbose, ignoreHostKey, multipleHosts, proxyEnabled, command, tunnelPort}
				if !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("flags = %v; esperado %v", got, tc.want)
				}
				if tunnelPort == 5000 && !c.Flags().Changed("port") {
					t.Fatal("--port não marcou a precedência sobre o YAML")
				}
			})
			if err != nil || !called {
				t.Fatalf("flags não aplicadas: %v", err)
			}
		})
	}
}

func TestRootDisplayFlags(t *testing.T) {
	for _, flag := range []string{"-s", "--servers", "-V", "--version"} {
		t.Run(flag, func(t *testing.T) {
			err := executeCLIForFlags(t, []string{flag}, func(_ *cobra.Command, _ []string) {
				wantServers := flag == "-s" || flag == "--servers"
				if showServers != wantServers || showVersion == wantServers {
					t.Fatal("flag de exibição ignorada")
				}
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUnsupportedSubcommandFlagsAreRejected(t *testing.T) {
	for _, args := range [][]string{
		{"--recursive", "cp", "up", "f", "server"},
		{"-l", "cp", "up", "f", "server"},
		{"cp", "up", "-c", "uptime", "f", "server"},
		{"-p", "cp", "up", "f", "server"},
		{"-s", "cp", "up", "f", "server"},
		{"cp", "down", "-l", "server", "remote"},
		{"port-forward", "-r", "server", "8080:80"},
		{"tunnel", "-l", "-j1"},
		{"tunnel", "status", "-u1"},
		{"tunnel", "stop", "-v"},
		{"update", "-u1"},
		{"man", "-j2"},
		{"cp", "up", "--desconhecida", "f", "server"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			err := executeCLIForFlags(t, args, func(_ *cobra.Command, _ []string) { t.Fatal("flag sem efeito foi aceita") })
			if err == nil {
				t.Fatal("esperado erro de flag incompatível")
			}
		})
	}
}

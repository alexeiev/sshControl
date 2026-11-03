package main

import (
	"testing"

	"github.com/spf13/cobra"
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

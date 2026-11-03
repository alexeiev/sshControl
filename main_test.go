package main

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestIgnoreHostKeyFlagIsInheritedByAllSSHCommands(t *testing.T) {
	for _, command := range []*cobra.Command{rootCmd, cpDownCmd, cpUpCmd, pfCmd, tunnelCmd} {
		t.Run(command.CommandPath(), func(t *testing.T) {
			ignoreHostKey = false
			t.Cleanup(func() { ignoreHostKey = false })
			if err := command.ParseFlags([]string{"--ignore-host-key"}); err != nil {
				t.Fatal(err)
			}
			if !ignoreHostKey {
				t.Fatal("flag não propagada")
			}
		})
	}
}

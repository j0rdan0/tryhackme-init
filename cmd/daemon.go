package cmd

import (
	"github.com/spf13/cobra"
	"extend-vm/pkg/vm"
)

var daemonCmd = &cobra.Command{
	Use:    "daemon [room_name]",
	Short:  "Internal use only: runs the auto-extend daemon loop",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		roomName := args[0]
		vm.LoopExtend(roomName)
	},
}

func init() {
	rootCmd.AddCommand(daemonCmd)
}

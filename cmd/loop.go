package cmd

import (
	"github.com/spf13/cobra"
	"extend-vm/pkg/vm"
)

var loopCmd = &cobra.Command{
	Use:   "loop [room_name]",
	Short: "Keep extending the VM every 30 minutes (daemon mode)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		roomName := args[0]
		vm.LoopExtend(roomName)
	},
}

func init() {
	rootCmd.AddCommand(loopCmd)
}

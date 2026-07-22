package cmd

import (
	"github.com/spf13/cobra"
	"extend-vm/pkg/vm"
)

var loopCmd = &cobra.Command{
	Use:   "loop [room_name]",
	Short: "Start the target VM, configure VPN/scan, and run the auto-extend daemon in the background",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		roomName := args[0]
		vm.Start(roomName, true)
	},
}

func init() {
	rootCmd.AddCommand(loopCmd)
}

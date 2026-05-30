package cmd

import (
	"github.com/spf13/cobra"
	"extend-vm/pkg/vm"
)

var startCmd = &cobra.Command{
	Use:   "start [room_name]",
	Short: "Start the target VM in the room",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		roomName := args[0]
		vm.Start(roomName)
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
}

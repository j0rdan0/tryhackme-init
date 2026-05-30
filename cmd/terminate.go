package cmd

import (
	"github.com/spf13/cobra"
	"extend-vm/pkg/vm"
)

var terminateCmd = &cobra.Command{
	Use:   "terminate [room_name]",
	Short: "Terminate the VM in the room",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		roomName := args[0]
		vm.Terminate(roomName)
	},
}

func init() {
	rootCmd.AddCommand(terminateCmd)
}

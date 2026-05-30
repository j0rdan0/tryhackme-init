package cmd

import (
	"github.com/spf13/cobra"
	"extend-vm/pkg/vm"
)

var extendCmd = &cobra.Command{
	Use:   "extend [room_name]",
	Short: "Add 1 hour to the VM duration",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		roomName := args[0]
		vm.Extend(roomName)
	},
}

func init() {
	rootCmd.AddCommand(extendCmd)
}

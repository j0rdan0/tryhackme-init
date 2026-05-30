package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"extend-vm/pkg/browser"
)

var rootCmd = &cobra.Command{
	Use:   "init-vm [room_name] [action] OR init-vm [command] [room_name]",
	Short: "Automate starting, extending, and terminating TryHackMe VMs.",
	Args:  cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// If they use the legacy syntax: ./init-vm cheesectfv10 start
		if len(args) >= 1 {
			roomName := args[0]
			action := "start"
			if len(args) > 1 {
				action = args[1]
			}
			
			switch action {
			case "start":
				startCmd.Run(startCmd, []string{roomName})
			case "extend":
				extendCmd.Run(extendCmd, []string{roomName})
			case "terminate":
				terminateCmd.Run(terminateCmd, []string{roomName})
			case "loop":
				loopCmd.Run(loopCmd, []string{roomName})
			default:
				fmt.Printf("Unknown action: %s. Usage: init-vm <room_name> [start|extend|terminate|loop]\n", action)
				os.Exit(1)
			}
			return
		}
		
		// If no args, print help
		_ = cmd.Help()
	},
}

func Execute() {
	browser.CleanScreenshots()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

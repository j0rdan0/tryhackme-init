package hosts

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

func UpdateEntry(roomName string, ip string) error {
	hostsPath := "/etc/hosts"
	hostname := roomName + ".thm"
	entry := fmt.Sprintf("%s\t%s\t# Added by init-vm TryHackMe tool", ip, hostname)

	// Read existing hosts file
	data, err := os.ReadFile(hostsPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", hostsPath, err)
	}

	lines := strings.Split(string(data), "\n")
	newLines := []string{}
	found := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			newLines = append(newLines, line)
			continue
		}
		parts := strings.Fields(trimmed)
		if len(parts) >= 2 && parts[1] == hostname {
			newLines = append(newLines, entry)
			found = true
		} else {
			newLines = append(newLines, line)
		}
	}

	if !found {
		if len(newLines) > 0 && newLines[len(newLines)-1] != "" {
			newLines = append(newLines, "")
		}
		newLines = append(newLines, entry)
	}

	newContent := strings.Join(newLines, "\n")
	if !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}

	// Try writing directly first
	err = os.WriteFile(hostsPath, []byte(newContent), 0644)
	if err == nil {
		fmt.Printf("Updated /etc/hosts: %s -> %s\n", hostname, ip)
		return nil
	}

	// If permission denied, write via sudo tee
	cmd := exec.Command("sudo", "tee", hostsPath)
	cmd.Stdout = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	_, _ = stdin.Write([]byte(newContent))
	stdin.Close()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("sudo tee failed: %w (please run with sudo or verify permissions)", err)
	}

	fmt.Printf("Updated /etc/hosts (via sudo): %s -> %s\n", hostname, ip)
	return nil
}

func RemoveEntry(roomName string) error {
	hostsPath := "/etc/hosts"
	hostname := roomName + ".thm"

	// Read existing hosts file
	data, err := os.ReadFile(hostsPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", hostsPath, err)
	}

	lines := strings.Split(string(data), "\n")
	newLines := []string{}
	removed := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			newLines = append(newLines, line)
			continue
		}
		parts := strings.Fields(trimmed)
		if len(parts) >= 2 && parts[1] == hostname {
			removed = true
			continue // Skip this line to remove it
		}
		newLines = append(newLines, line)
	}

	if !removed {
		return nil
	}

	newContent := strings.Join(newLines, "\n")
	if !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}

	// Try writing directly first
	err = os.WriteFile(hostsPath, []byte(newContent), 0644)
	if err == nil {
		fmt.Printf("Removed %s from /etc/hosts\n", hostname)
		return nil
	}

	// If permission denied, write via sudo tee
	cmd := exec.Command("sudo", "tee", hostsPath)
	cmd.Stdout = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	_, _ = stdin.Write([]byte(newContent))
	stdin.Close()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("sudo tee failed: %w (please run with sudo or verify permissions)", err)
	}

	fmt.Printf("Removed %s from /etc/hosts (via sudo)\n", hostname)
	return nil
}

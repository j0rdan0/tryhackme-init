package vm

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/joakimcarlsson/bonk"
	"extend-vm/pkg/browser"
	"extend-vm/pkg/hosts"
)

var SESSION_FILE = filepath.Join(os.TempDir(), "session.dat")

func Start(roomName string) {
	roomURL := "https://tryhackme.com/room/" + roomName
	hasSession := false
	if _, err := os.Stat(SESSION_FILE); err == nil {
		hasSession = true
	}

	// Run headlessly if we have a session file
	b, ctx, page, err := browser.Launch(hasSession)
	if err != nil {
		log.Fatal(err)
	}
	defer b.Close()

	// Navigate with a 15-second timeout
	fmt.Printf("Navigating to %s...\n", roomURL)
	err = page.Navigate(roomURL, bonk.WithTimeout(15*time.Second))
	if err != nil {
		fmt.Printf("Navigation error: %v\n", err)
	}
	fmt.Println("Navigated to room page")

	time.Sleep(3 * time.Second)

	// Capture a screenshot of the headless run to verify what is loaded
	if hasSession {
		_ = page.Screenshot(filepath.Join(os.TempDir(), "headless-screenshot.png"))
		fmt.Println("Captured headless-screenshot.png to verify login state.")
	}

	// Step 1: Check if we are logged in.
	isLoggedInVal, err := page.Timeout(3 * time.Second).Evaluate(`(() => {
		return !!(
			document.querySelector('.profile-avatar') || 
			document.querySelector('.nav-user') || 
			document.querySelector('#start-attackbox') ||
			document.querySelector('#active-machine-info') ||
			document.querySelector('[id^="start-machine-button"]')
		);
	})()`)

	isLoggedIn := false
	if err == nil {
		isLoggedIn = isLoggedInVal.(bool)
	}

	if isLoggedIn {
		fmt.Println("Session is valid! Checking target VM status...")

		// Check if VM is already active
		hasActiveVM, _ := page.Evaluate(`(() => {
			const activeInfo = document.getElementById('active-machine-info');
			if (!activeInfo) return false;
			const text = activeInfo.innerText.toLowerCase();
			return text.includes("terminate") || text.includes("add 1 hour") || /\b10\.\d+\.\d+\.\d+\b/.test(text);
		})()`)
		if hasActiveVM.(bool) {
			fmt.Println("Target VM is already running.")
			if ip := logTargetIP(page, roomName); ip != "" {
				startVPNAndScan(roomName, ip)
			}
			fmt.Printf("Saving updated session state to %s...\n", SESSION_FILE)
			_ = ctx.SaveState(SESSION_FILE)
			browser.CleanScreenshots()
			return
		}

		// Try clicking the start machine button automatically using JS click injection.
		fmt.Println("Checking for start machine button...")
		clicked, clickErr := page.Timeout(10 * time.Second).Evaluate(`(async () => {
			// 1. Wait for header-1 to render in the DOM (polling for up to 5 seconds)
			await new Promise((resolve) => {
				const timeout = setTimeout(resolve, 5000);
				const check = () => {
					if (document.getElementById('header-1')) {
						clearTimeout(timeout);
						resolve();
					} else {
						setTimeout(check, 250);
					}
				};
				check();
			});

			// 2. Expand header-1 if collapsed
			const h = document.getElementById('header-1');
			if (h) {
				const expanded = h.getAttribute('aria-expanded');
				if (expanded === 'false' || expanded === null) {
					h.click();
				}
			}
			
			// Wait 1s for DOM / React rendering
			await new Promise(resolve => setTimeout(resolve, 1000));

			// 3. Find and click the start machine button
			const btn = document.getElementById('start-machine-button-1') || 
			            document.querySelector('[id^="start-machine-button"]');
			if (btn) {
				btn.click();
				return true;
			}
			return false;
		})()`)

		if clickErr == nil && clicked.(bool) {
			fmt.Println("Successfully clicked start machine button automatically!")
			if ip := logTargetIP(page, roomName); ip != "" {
				startVPNAndScan(roomName, ip)
			}
			fmt.Printf("Saving updated session state to %s...\n", SESSION_FILE)
			_ = ctx.SaveState(SESSION_FILE)
			fmt.Println("Done!")
			browser.CleanScreenshots()
			return
		}

		fmt.Println("Warning: Logged in, but target machine could not be started automatically (button not found and VM not running).")
	}

	// If we are NOT logged in and we were running headlessly, relaunch headfully
	if !isLoggedIn && hasSession {
		fmt.Println("Session expired or invalid. Relaunching in headful mode to allow login...")
		b.Close() // Close headless browser

		b, ctx, page, err = browser.Launch(false)
		if err != nil {
			log.Fatal(err)
		}
		defer b.Close()

		fmt.Printf("Navigating to %s (headful)...\n", roomURL)
		_ = page.Navigate(roomURL, bonk.WithTimeout(15*time.Second))
		time.Sleep(3 * time.Second)
	}

	// Capture a debug screenshot of the headful window just in case
	_ = page.Screenshot(filepath.Join(os.TempDir(), "debug-screenshot.png"))

	// Prompt the user to log in
	fmt.Println("\n============================================================")
	fmt.Println("INSTRUCTIONS:")
	fmt.Println("1. If you are not logged in, please log in now in the browser window.")
	fmt.Println("2. Once logged in, the VM will be ready to start.")
	fmt.Println("3. Press [ENTER] or [Ctrl+C] in this terminal to save session & start VM.")
	fmt.Println("============================================================")

	done := make(chan bool, 1)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n[Ctrl+C] detected. Saving state and exiting...")
		done <- true
	}()

	// Wait for Enter key on /dev/tty
	go func() {
		var reader *bufio.Reader
		if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
			defer tty.Close()
			reader = bufio.NewReader(tty)
		} else {
			reader = bufio.NewReader(os.Stdin)
		}

		for {
			_, err := reader.ReadString('\n')
			if err == nil {
				done <- true
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	}()

	<-done

	// Now try clicking the button again using JS click injection after expanding
	fmt.Println("Attempting to click start machine button after login...")
	val, err := page.Timeout(10 * time.Second).Evaluate(`(async () => {
		// 1. Wait for header-1 to render in the DOM (polling for up to 5 seconds)
		await new Promise((resolve) => {
			const timeout = setTimeout(resolve, 5000);
			const check = () => {
				if (document.getElementById('header-1')) {
					clearTimeout(timeout);
					resolve();
				} else {
					setTimeout(check, 250);
				}
			};
			check();
		});

		// 2. Expand header-1 if collapsed
		const h = document.getElementById('header-1');
		if (h) {
			const expanded = h.getAttribute('aria-expanded');
			if (expanded === 'false' || expanded === null) {
				h.click();
			}
		}
		
		// Wait 1s for DOM / React rendering
		await new Promise(resolve => setTimeout(resolve, 1000));

		// 3. Find and click the start machine button
		const btn = document.getElementById('start-machine-button-1') || 
		            document.querySelector('[id^="start-machine-button"]');
		if (btn) {
			btn.click();
			return true;
		}
		return false;
	})()`)

	if err != nil || val == false {
		fmt.Printf("Warning: Click failed after login (button not found or clicked): %v (found: %v)\n", err, val)
	} else {
		fmt.Println("Clicked start machine button successfully!")
		if ip := logTargetIP(page, roomName); ip != "" {
			startVPNAndScan(roomName, ip)
		}
	}

	// Save session state
	fmt.Printf("Saving current session state to %s...\n", SESSION_FILE)
	err = ctx.SaveState(SESSION_FILE)
	if err != nil {
		fmt.Printf("Error saving session state: %v\n", err)
	} else {
		fmt.Println("Session state saved successfully! You can reuse it in future runs.")
	}

	browser.CleanScreenshots()
	time.Sleep(1 * time.Second)
}

func Extend(roomName string) {
	roomURL := "https://tryhackme.com/room/" + roomName
	hasSession := false
	if _, err := os.Stat(SESSION_FILE); err == nil {
		hasSession = true
	}

	b, ctx, page, err := browser.Launch(hasSession)
	if err != nil {
		log.Fatal(err)
	}
	defer b.Close()

	// Navigate with a 15-second timeout
	fmt.Printf("Navigating to %s...\n", roomURL)
	err = page.Navigate(roomURL, bonk.WithTimeout(15*time.Second))
	if err != nil {
		fmt.Printf("Navigation error: %v\n", err)
	}
	fmt.Println("Navigated to room page")

	time.Sleep(3 * time.Second)

	// Step 1: Check if we are logged in.
	isLoggedInVal, err := page.Timeout(3 * time.Second).Evaluate(`(() => {
		return !!(
			document.querySelector('.profile-avatar') || 
			document.querySelector('.nav-user') || 
			document.querySelector('#start-attackbox') ||
			document.querySelector('#active-machine-info') ||
			document.querySelector('[id^="start-machine-button"]')
		);
	})()`)

	isLoggedIn := false
	if err == nil {
		isLoggedIn = isLoggedInVal.(bool)
	}

	if isLoggedIn {
		fmt.Println("Session is valid! Checking target VM status...")

		// Check if VM is active
		hasActiveVM, _ := page.Evaluate(`(() => {
			const activeInfo = document.getElementById('active-machine-info');
			if (!activeInfo) return false;
			const text = activeInfo.innerText.toLowerCase();
			return text.includes("terminate") || text.includes("add 1 hour") || /\b10\.\d+\.\d+\.\d+\b/.test(text);
		})()`)
		if !hasActiveVM.(bool) {
			fmt.Println("Target VM is not running. Cannot extend VM.")
			fmt.Printf("Saving updated session state to %s...\n", SESSION_FILE)
			_ = ctx.SaveState(SESSION_FILE)
			browser.CleanScreenshots()
			return
		}

		// Attempt to click the "Add 1 hour" button
		fmt.Println("Attempting to click 'Add 1 hour' button automatically...")
		err = page.Timeout(5 * time.Second).GetByText("Add 1 hour").Click()
		if err == nil {
			fmt.Println("Successfully clicked 'Add 1 hour' button automatically!")
			fmt.Printf("Saving updated session state to %s...\n", SESSION_FILE)
			_ = ctx.SaveState(SESSION_FILE)
			browser.CleanScreenshots()
			time.Sleep(2 * time.Second)
			return
		}
		fmt.Println("Warning: Logged in, but failed to click 'Add 1 hour' button automatically.")
	}

	// If click failed and we were headless, relaunch headfully
	if !isLoggedIn && hasSession {
		fmt.Println("Session expired or invalid. Relaunching in headful mode to allow login...")
		b.Close() // Close headless browser

		b, ctx, page, err = browser.Launch(false)
		if err != nil {
			log.Fatal(err)
		}
		defer b.Close()

		fmt.Printf("Navigating to %s (headful)...\n", roomURL)
		_ = page.Navigate(roomURL, bonk.WithTimeout(15*time.Second))
		time.Sleep(3 * time.Second)
	}

	// Prompt the user to log in
	fmt.Println("\n============================================================")
	fmt.Println("INSTRUCTIONS:")
	fmt.Println("1. If you are not logged in, please log in now in the browser window.")
	fmt.Println("2. Press [ENTER] or [Ctrl+C] in this terminal to save session & extend VM.")
	fmt.Println("============================================================")

	done := make(chan bool, 1)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		done <- true
	}()

	go func() {
		var reader *bufio.Reader
		if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
			defer tty.Close()
			reader = bufio.NewReader(tty)
		} else {
			reader = bufio.NewReader(os.Stdin)
		}
		for {
			_, err := reader.ReadString('\n')
			if err == nil {
				done <- true
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	}()

	<-done

	// Check if VM is active after login
	hasActiveVMPost, _ := page.Evaluate(`(() => {
		const activeInfo = document.getElementById('active-machine-info');
		if (!activeInfo) return false;
		const text = activeInfo.innerText.toLowerCase();
		return text.includes("terminate") || text.includes("add 1 hour") || /\b10\.\d+\.\d+\.\d+\b/.test(text);
	})()`)

	if !hasActiveVMPost.(bool) {
		fmt.Println("Target VM is not running. Cannot extend VM.")
		fmt.Printf("Saving updated session state to %s...\n", SESSION_FILE)
		_ = ctx.SaveState(SESSION_FILE)
		browser.CleanScreenshots()
		return
	}

	fmt.Println("Attempting to click 'Add 1 hour' button after login...")
	err = page.Timeout(5 * time.Second).GetByText("Add 1 hour").Click()
	if err != nil {
		fmt.Printf("Error: Could not click 'Add 1 hour' button: %v\n", err)
	} else {
		fmt.Println("Successfully clicked 'Add 1 hour' button!")
	}

	fmt.Printf("Saving updated session state to %s...\n", SESSION_FILE)
	_ = ctx.SaveState(SESSION_FILE)
	browser.CleanScreenshots()
	time.Sleep(2 * time.Second)
}

func Terminate(roomName string) {
	// Call vpn.sh stop script
	fmt.Println("Stopping VPN...")
	if err := runVPN("stop"); err != nil {
		fmt.Printf("Warning: failed to stop VPN: %v\n", err)
	}

	roomURL := "https://tryhackme.com/room/" + roomName
	hasSession := false
	if _, err := os.Stat(SESSION_FILE); err == nil {
		hasSession = true
	}

	b, ctx, page, err := browser.Launch(hasSession)
	if err != nil {
		log.Fatal(err)
	}
	defer b.Close()

	// Navigate with a 15-second timeout
	fmt.Printf("Navigating to %s...\n", roomURL)
	err = page.Navigate(roomURL, bonk.WithTimeout(15*time.Second))
	if err != nil {
		fmt.Printf("Navigation error: %v\n", err)
	}
	fmt.Println("Navigated to room page")

	time.Sleep(3 * time.Second)

	// Step 1: Check if we are logged in.
	isLoggedInVal, err := page.Timeout(3 * time.Second).Evaluate(`(() => {
		return !!(
			document.querySelector('.profile-avatar') || 
			document.querySelector('.nav-user') || 
			document.querySelector('#start-attackbox') ||
			document.querySelector('#active-machine-info') ||
			document.querySelector('[id^="start-machine-button"]')
		);
	})()`)

	isLoggedIn := false
	if err == nil {
		isLoggedIn = isLoggedInVal.(bool)
	}

	if isLoggedIn {
		fmt.Println("Session is valid! Checking target VM status...")

		// Check if VM is active
		hasActiveVM, _ := page.Evaluate(`(() => {
			const activeInfo = document.getElementById('active-machine-info');
			if (!activeInfo) return false;
			const text = activeInfo.innerText.toLowerCase();
			return text.includes("terminate") || text.includes("add 1 hour") || /\b10\.\d+\.\d+\.\d+\b/.test(text);
		})()`)
		if !hasActiveVM.(bool) {
			fmt.Println("Target VM is not running. Nothing to terminate.")
			_ = hosts.RemoveEntry(roomName)
			fmt.Printf("Saving updated session state to %s...\n", SESSION_FILE)
			_ = ctx.SaveState(SESSION_FILE)
			browser.CleanScreenshots()
			return
		}

		// Proceed to terminate the active VM
		fmt.Println("Locating 'Terminate' button...")
		val, err := page.Evaluate(`(() => {
			const btns = Array.from(document.querySelectorAll('button')).filter(b => b.innerText.toLowerCase().includes("terminate"));
			if (btns.length > 0) {
				btns[0].click();
				return true;
			}
			return false;
		})()`)

		if err == nil && val == true {
			fmt.Println("Clicked initial 'Terminate' button. Confirming...")
			time.Sleep(1 * time.Second)
			valConf, errConf := page.Evaluate(`(() => {
				const btns = Array.from(document.querySelectorAll('button')).filter(b => {
					const style = window.getComputedStyle(b);
					return b.innerText.toLowerCase().includes("terminate") && style.display !== 'none' && style.visibility !== 'hidden';
				});
				if (btns.length > 1) {
					btns[btns.length - 1].click(); // Confirmation modal button
					return true;
				} else if (btns.length === 1) {
					btns[0].click();
					return true;
				}
				return false;
			})()`)
			if errConf == nil && valConf == true {
				fmt.Println("Successfully clicked confirmation button! Terminating VM...")
				_ = hosts.RemoveEntry(roomName)
				fmt.Printf("Saving updated session state to %s...\n", SESSION_FILE)
				_ = ctx.SaveState(SESSION_FILE)
				browser.CleanScreenshots()
				time.Sleep(3 * time.Second)
				return
			}
		}
		fmt.Println("Warning: Logged in, but failed to click 'Terminate' button automatically.")
	}

	// If we are NOT logged in and we were running headlessly, relaunch headfully
	if !isLoggedIn && hasSession {
		fmt.Println("Session expired or invalid. Relaunching in headful mode to allow login...")
		b.Close() // Close headless browser

		b, ctx, page, err = browser.Launch(false)
		if err != nil {
			log.Fatal(err)
		}
		defer b.Close()

		fmt.Printf("Navigating to %s (headful)...\n", roomURL)
		_ = page.Navigate(roomURL, bonk.WithTimeout(15*time.Second))
		time.Sleep(3 * time.Second)
	}

	// Prompt the user to log in
	fmt.Println("\n============================================================")
	fmt.Println("INSTRUCTIONS:")
	fmt.Println("1. If you are not logged in, please log in now in the browser window.")
	fmt.Println("2. Press [ENTER] or [Ctrl+C] in this terminal to save session & terminate VM.")
	fmt.Println("============================================================")

	done := make(chan bool, 1)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		done <- true
	}()

	go func() {
		var reader *bufio.Reader
		if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
			defer tty.Close()
			reader = bufio.NewReader(tty)
		} else {
			reader = bufio.NewReader(os.Stdin)
		}
		for {
			_, err := reader.ReadString('\n')
			if err == nil {
				done <- true
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	}()

	<-done

	// Check if VM is active after login
	hasActiveVMPost, _ := page.Evaluate(`(() => {
		const activeInfo = document.getElementById('active-machine-info');
		if (!activeInfo) return false;
		const text = activeInfo.innerText.toLowerCase();
		return text.includes("terminate") || text.includes("add 1 hour") || /\b10\.\d+\.\d+\.\d+\b/.test(text);
	})()`)

	if !hasActiveVMPost.(bool) {
		fmt.Println("Target VM is not running. Nothing to terminate.")
		_ = hosts.RemoveEntry(roomName)
		fmt.Printf("Saving updated session state to %s...\n", SESSION_FILE)
		_ = ctx.SaveState(SESSION_FILE)
		browser.CleanScreenshots()
		return
	}

	// Step 1: Click the initial Terminate button again after login
	fmt.Println("Locating 'Terminate' button after login...")
	_, _ = page.Evaluate(`(() => {
		const btns = Array.from(document.querySelectorAll('button')).filter(b => b.innerText.toLowerCase().includes("terminate"));
		if (btns.length > 0) {
			btns[0].click();
			return true;
		}
		return false;
	})()`)

	time.Sleep(1 * time.Second)

	// Step 2: Confirm
	fmt.Println("Confirming termination after login...")
	_, _ = page.Evaluate(`(() => {
		const btns = Array.from(document.querySelectorAll('button')).filter(b => {
			const style = window.getComputedStyle(b);
			return b.innerText.toLowerCase().includes("terminate") && style.display !== 'none' && style.visibility !== 'hidden';
		});
		if (btns.length > 1) {
			btns[btns.length - 1].click();
			return true;
		} else if (btns.length === 1) {
			btns[0].click();
			return true;
		}
		return false;
	})()`)

	fmt.Printf("Saving updated session state to %s...\n", SESSION_FILE)
	_ = ctx.SaveState(SESSION_FILE)
	_ = hosts.RemoveEntry(roomName)
	browser.CleanScreenshots()
	time.Sleep(3 * time.Second)
	fmt.Println("Done!")
}

// LoopExtend runs Extend every 30 minutes until terminated with Ctrl+C
func LoopExtend(roomName string) {
	fmt.Printf("Starting auto-extend daemon for room '%s' (running every 30 minutes)...\n", roomName)
	fmt.Println("Press [Ctrl+C] to stop this loop.")

	// Perform initial extension immediately
	Extend(roomName)

	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	// Capture interrupt signal to shut down gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-sigChan:
			fmt.Println("\nAuto-extend daemon stopped.")
			return
		case <-ticker.C:
			fmt.Printf("[%s] Running scheduled extension...\n", time.Now().Format("15:04:05"))
			Extend(roomName)
		}
	}
}

// logTargetIP polls the webpage to find the assigned 10.x.x.x IP address of the VM
func logTargetIP(page *bonk.Page, roomName string) string {
	fmt.Println("Checking for target IP address...")

	// Fast check first
	val, err := page.Evaluate(`(() => {
		const el = document.getElementById('active-machine-info') || document.body;
		const match = el.innerText.match(/\b(10\.\d+\.\d+\.\d+)\b/);
		return match ? match[1] : "";
	})()`)
	if err == nil {
		if ip, ok := val.(string); ok && ip != "" {
			printIP(ip, roomName)
			_ = hosts.UpdateEntry(roomName, ip)
			return ip
		}
	}

	// If not found immediately, poll for up to 90 seconds (useful during initial boot)
	fmt.Println("Waiting for IP address to be assigned (polling for up to 90s)...")
	for i := 0; i < 45; i++ {
		val, err := page.Evaluate(`(() => {
			const el = document.getElementById('active-machine-info') || document.body;
			const match = el.innerText.match(/\b(10\.\d+\.\d+\.\d+)\b/);
			return match ? match[1] : "";
		})()`)
		if err == nil {
			if ip, ok := val.(string); ok && ip != "" {
				printIP(ip, roomName)
				_ = hosts.UpdateEntry(roomName, ip)
				return ip
			}
		}
		time.Sleep(2 * time.Second)
	}
	fmt.Println("No active target IP address detected on the page.")
	return ""
}

func printIP(ip string, roomName string) {
	fmt.Printf("\n============================================================\n")
	fmt.Printf("TARGET VM IP ADDRESS: %s\n", ip)
	fmt.Printf("DNS RECORD (local):   %s.thm\n", roomName)
	fmt.Printf("============================================================\n\n")
}

func getWorkspaceDir() (string, error) {
	// Try to find vpn.sh in various directories
	searchDirs := []string{".", ".."}

	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		searchDirs = append(searchDirs, exeDir, filepath.Dir(exeDir))
	}

	for _, dir := range searchDirs {
		p := filepath.Join(dir, "vpn.sh")
		if _, err := os.Stat(p); err == nil {
			absPath, err := filepath.Abs(p)
			if err != nil {
				return "", err
			}
			return filepath.Dir(absPath), nil
		}
	}
	return "", fmt.Errorf("vpn.sh not found in search paths")
}

func runVPN(action string) error {
	workspaceDir, err := getWorkspaceDir()
	if err != nil {
		return err
	}

	vpnPath := filepath.Join(workspaceDir, "vpn.sh")
	cmd := exec.Command(vpnPath, action)
	cmd.Dir = workspaceDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runScan(roomName string, ip string) {
	fmt.Printf("Starting scan.sh with IP %s...\n", ip)
	workspaceDir, err := getWorkspaceDir()
	if err != nil {
		fmt.Printf("Warning: could not determine workspace directory for scan: %v\n", err)
		return
	}

	scanPath := filepath.Join(workspaceDir, "scan.sh")
	if _, err := os.Stat(scanPath); err != nil {
		fmt.Printf("Warning: scan.sh not found at %s: %v\n", scanPath, err)
		return
	}

	roomDir := filepath.Join(workspaceDir, roomName)
	if err := os.MkdirAll(roomDir, 0755); err != nil {
		fmt.Printf("Warning: failed to create room directory %s: %v\n", roomDir, err)
		return
	}

	cmd := exec.Command(scanPath, ip)
	cmd.Dir = roomDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("Warning: scan.sh failed: %v\n", err)
	}
}

func startVPNAndScan(roomName string, ip string) {
	fmt.Println("Starting VPN...")
	if err := runVPN("start"); err != nil {
		fmt.Printf("Warning: failed to start VPN: %v\n", err)
	}
	runScan(roomName, ip)
}

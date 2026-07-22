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

	"extend-vm/pkg/browser"
	"extend-vm/pkg/hosts"

	"github.com/joakimcarlsson/bonk"
)

func checkLoggedIn(page *bonk.Page) bool {
	// Poll for up to 15 seconds to wait for page initialization (spinner to disappear)
	val, err := page.Timeout(15 * time.Second).Evaluate(`(async () => {
		for (let i = 0; i < 60; i++) {
			if (
				document.querySelector('.profile-avatar') || 
				document.querySelector('.nav-user') || 
				document.querySelector('#start-attackbox') ||
				document.querySelector('#active-machine-info') ||
				document.querySelector('[id^="active-machines-information"]') ||
				document.querySelector('[id^="start-machine-button"]:not([disabled])')
			) {
				return true;
			}
			if (
				window.location.pathname.includes('/login') ||
				document.querySelector('a[href*="/login"]') ||
				Array.from(document.querySelectorAll('a, button')).some(el => {
					const txt = el.innerText.toLowerCase();
					return txt.includes('log in') || txt.includes('sign in') || txt.includes('login') || txt.includes('signin');
				})
			) {
				return false;
			}
			await new Promise(resolve => setTimeout(resolve, 250));
		}
		return false;
	})()`)
	if err != nil {
		return false
	}
	res, ok := val.(bool)
	return ok && res
}

func checkLoggedInWithTimeout(page *bonk.Page, timeout time.Duration) bool {
	isLoggedInVal, err := page.Timeout(timeout).Evaluate(`(() => {
		return !!(
			document.querySelector('.profile-avatar') || 
			document.querySelector('.nav-user') || 
			document.querySelector('#start-attackbox') ||
			document.querySelector('#active-machine-info') ||
			document.querySelector('[id^="active-machines-information"]') ||
			document.querySelector('[id^="start-machine-button"]:not([disabled])')
		);
	})()`)
	if err != nil {
		return false
	}
	val, ok := isLoggedInVal.(bool)
	return ok && val
}

func Start(roomName string, enableLoop bool) {
	roomURL := "https://tryhackme.com/room/" + roomName
	hasSession := false
	if _, err := os.Stat(browser.SESSION_FILE); err == nil {
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

	isLoggedIn := checkLoggedIn(page)


	// If we are NOT logged in and we were running headlessly, relaunch headfully
	if !isLoggedIn && hasSession {
		fmt.Println("Session expired, invalid or blocked. Relaunching in headful mode to allow login...")
		b.Close() // Close headless browser

		b, ctx, page, err = browser.Launch(false)
		if err != nil {
			log.Fatal(err)
		}
		defer b.Close()

		fmt.Printf("Navigating to %s (headful)...\n", roomURL)
		_ = page.Navigate(roomURL, bonk.WithTimeout(15*time.Second))
		time.Sleep(3 * time.Second)

		isLoggedIn = checkLoggedIn(page)
	}

	// Capture a debug screenshot of the headful window just in case
	_ = page.Screenshot(filepath.Join(os.TempDir(), "debug-screenshot.png"))

	if !isLoggedIn {
		// Prompt the user to log in
		fmt.Println("\n============================================================")
		fmt.Println("INSTRUCTIONS:")
		fmt.Println("1. Please log in now in the browser window.")
		fmt.Println("2. Once logged in, the VM will be ready to start.")
		fmt.Println("3. The script will automatically detect once you are logged in, or")
		fmt.Println("   you can press [ENTER] in this terminal to force continue.")
		fmt.Println("============================================================")

		done := make(chan bool, 1)
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigChan
			fmt.Println("\n[Ctrl+C] detected. Saving state and exiting...")
			done <- true
		}()

		// Goroutine to poll login status
		go func() {
			for {
				select {
				case <-done:
					return
				default:
					if page.IsClosed() {
						return
					}
					if checkLoggedInWithTimeout(page, 1*time.Second) {
						fmt.Println("\nLogin detected automatically!")
						done <- true
						return
					}
					time.Sleep(1500 * time.Millisecond)
				}
			}
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

			_, _ = reader.ReadString('\n')
			done <- true
		}()

		<-done
	}

	// Check if VM is already active
	hasActiveVM, _ := page.Evaluate(`(() => {
		const activeInfo = document.getElementById('active-machine-info') || 
		                   document.getElementById('active-machines-information-header') ||
		                   document.querySelector('[id^="active-machines-information"]') ||
		                   document.querySelector('[class*="active-machines-information"]');
		if (activeInfo) {
			const text = activeInfo.innerText.toLowerCase();
			if (text.includes("terminate") || text.includes("add 1 hour") || /\b10\.\d+\.\d+\.\d+\b/.test(text)) {
				return true;
			}
		}
		const btns = Array.from(document.querySelectorAll('button'));
		const hasTerminate = btns.some(b => b.innerText.toLowerCase().includes("terminate"));
		const hasAddHour = btns.some(b => b.innerText.toLowerCase().includes("add 1 hour"));
		return hasTerminate || hasAddHour;
	})()`)
	if hasActiveVM.(bool) {
		fmt.Println("Target VM is already running.")
		if ip := logTargetIP(page, roomName); ip != "" {
			startVPNAndScan(roomName, ip)
		}
		fmt.Printf("Saving updated session state to %s...\n", browser.SESSION_FILE)
		_ = ctx.SaveState(browser.SESSION_FILE)
		browser.CleanScreenshots()
		if enableLoop {
			startLoopDaemon(roomName)
		}
		dropIntoShell(roomName)
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
			if (btn.disabled) {
				return false;
			}
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
	} else {
		fmt.Printf("Warning: Click failed (button not found or clicked): %v (found: %v)\n", clickErr, clicked)
	}

	// Save session state
	fmt.Printf("Saving current session state to %s...\n", browser.SESSION_FILE)
	err = ctx.SaveState(browser.SESSION_FILE)
	if err != nil {
		fmt.Printf("Error saving session state: %v\n", err)
	} else {
		fmt.Println("Session state saved successfully! You can reuse it in future runs.")
	}

	browser.CleanScreenshots()
	time.Sleep(1 * time.Second)
	if enableLoop {
		startLoopDaemon(roomName)
	}
	dropIntoShell(roomName)
}

func dropIntoShell(roomName string) {
	// Ensure we are in the room directory
	projectRoot := getProjectRoot()
	roomDir := filepath.Join(projectRoot, roomName)
	if fi, err := os.Stat(roomDir); err == nil && fi.IsDir() {
		_ = os.Chdir(roomDir)
	}
	
	if targetCwd, err := os.Getwd(); err == nil {
		fmt.Printf("Dropping into shell in: %s\n", targetCwd)
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	env := os.Environ()
	browser.CloseAll()
	_ = syscall.Exec(shell, []string{shell}, env)
}

func Extend(roomName string) {
	roomURL := "https://tryhackme.com/room/" + roomName
	hasSession := false
	if _, err := os.Stat(browser.SESSION_FILE); err == nil {
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

	isLoggedIn := checkLoggedIn(page)

	// If we are NOT logged in and we were running headlessly, relaunch headfully
	if !isLoggedIn && hasSession {
		fmt.Println("Session expired, invalid or blocked. Relaunching in headful mode to allow login...")
		b.Close() // Close headless browser

		b, ctx, page, err = browser.Launch(false)
		if err != nil {
			log.Fatal(err)
		}
		defer b.Close()

		fmt.Printf("Navigating to %s (headful)...\n", roomURL)
		_ = page.Navigate(roomURL, bonk.WithTimeout(15*time.Second))
		time.Sleep(3 * time.Second)

		isLoggedIn = checkLoggedIn(page)
	}

	// Capture a debug screenshot of the headful window just in case
	_ = page.Screenshot(filepath.Join(os.TempDir(), "debug-screenshot.png"))

	// If still not logged in, prompt user to log in
	if !isLoggedIn {
		fmt.Println("\n============================================================")
		fmt.Println("INSTRUCTIONS:")
		fmt.Println("1. Please log in now in the browser window.")
		fmt.Println("2. The script will automatically detect once you are logged in, or")
		fmt.Println("   you can press [ENTER] in this terminal to force continue.")
		fmt.Println("============================================================")

		done := make(chan bool, 1)
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigChan
			done <- true
		}()

		// Goroutine to poll login status
		go func() {
			for {
				select {
				case <-done:
					return
				default:
					if page.IsClosed() {
						return
					}
					if checkLoggedInWithTimeout(page, 1*time.Second) {
						fmt.Println("\nLogin detected automatically!")
						done <- true
						return
					}
					time.Sleep(1500 * time.Millisecond)
				}
			}
		}()

		// Goroutine to wait for manual Enter key
		go func() {
			var reader *bufio.Reader
			if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
				defer tty.Close()
				reader = bufio.NewReader(tty)
			} else {
				reader = bufio.NewReader(os.Stdin)
			}
			_, _ = reader.ReadString('\n')
			done <- true
		}()

		<-done
	}

	// Now we are logged in (or forced to continue)
	// Check if VM is active
	fmt.Println("Checking target VM status...")
	hasActiveVM, _ := page.Evaluate(`(() => {
		const activeInfo = document.getElementById('active-machine-info') || 
		                   document.getElementById('active-machines-information-header') ||
		                   document.querySelector('[id^="active-machines-information"]') ||
		                   document.querySelector('[class*="active-machines-information"]');
		if (activeInfo) {
			const text = activeInfo.innerText.toLowerCase();
			if (text.includes("terminate") || text.includes("add 1 hour") || /\b10\.\d+\.\d+\.\d+\b/.test(text)) {
				return true;
			}
		}
		const btns = Array.from(document.querySelectorAll('button'));
		const hasTerminate = btns.some(b => b.innerText.toLowerCase().includes("terminate"));
		const hasAddHour = btns.some(b => b.innerText.toLowerCase().includes("add 1 hour"));
		return hasTerminate || hasAddHour;
	})()`)

	if !hasActiveVM.(bool) {
		fmt.Println("Target VM is not running. Starting VM...")
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
				if (btn.disabled) {
					return false;
				}
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
		} else {
			fmt.Printf("Warning: Click failed (button not found or clicked): %v (found: %v)\n", clickErr, clicked)
		}
	} else {
		// Attempt to click the "Add 1 hour" button
		fmt.Println("Attempting to click 'Add 1 hour' button automatically...")
		clicked, clickErr := page.Timeout(5 * time.Second).Evaluate(`(() => {
			const els = Array.from(document.querySelectorAll('button, a, [role="button"]'));
			const btn = els.find(el => {
				const text = (el.innerText || el.textContent || "").toLowerCase();
				return text.includes("add 1 hour") || text.includes("add 1 hr");
			});
			if (btn) {
				btn.click();
				return true;
			}
			return false;
		})()`)
		if clickErr != nil {
			fmt.Printf("Error: Could not click 'Add 1 hour' button: %v\n", clickErr)
		} else if clickedBool, ok := clicked.(bool); !ok || !clickedBool {
			fmt.Println("Error: 'Add 1 hour' button not found on the page.")
		} else {
			fmt.Println("Successfully clicked 'Add 1 hour' button!")
		}
	}

	fmt.Printf("Saving updated session state to %s...\n", browser.SESSION_FILE)
	_ = ctx.SaveState(browser.SESSION_FILE)
	browser.CleanScreenshots()
	time.Sleep(2 * time.Second)
}

func Terminate(roomName string) {
	// Stop the loop daemon if running
	stopLoopDaemon(roomName)

	// Call vpn.sh stop script
	fmt.Println("Stopping VPN...")
	if err := runVPN("stop"); err != nil {
		fmt.Printf("Warning: failed to stop VPN: %v\n", err)
	}

	roomURL := "https://tryhackme.com/room/" + roomName
	hasSession := false
	if _, err := os.Stat(browser.SESSION_FILE); err == nil {
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

	isLoggedIn := checkLoggedIn(page)

	// If we are NOT logged in and we were running headlessly, relaunch headfully
	if !isLoggedIn && hasSession {
		fmt.Println("Session expired, invalid or blocked. Relaunching in headful mode to allow login...")
		b.Close() // Close headless browser

		b, ctx, page, err = browser.Launch(false)
		if err != nil {
			log.Fatal(err)
		}
		defer b.Close()

		fmt.Printf("Navigating to %s (headful)...\n", roomURL)
		_ = page.Navigate(roomURL, bonk.WithTimeout(15*time.Second))
		time.Sleep(3 * time.Second)

		isLoggedIn = checkLoggedIn(page)
	}

	// Capture a debug screenshot of the headful window just in case
	_ = page.Screenshot(filepath.Join(os.TempDir(), "debug-screenshot.png"))

	// If still not logged in, prompt user to log in
	if !isLoggedIn {
		fmt.Println("\n============================================================")
		fmt.Println("INSTRUCTIONS:")
		fmt.Println("1. Please log in now in the browser window.")
		fmt.Println("2. The script will automatically detect once you are logged in, or")
		fmt.Println("   you can press [ENTER] in this terminal to force continue.")
		fmt.Println("============================================================")

		done := make(chan bool, 1)
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigChan
			done <- true
		}()

		// Goroutine to poll login status
		go func() {
			for {
				select {
				case <-done:
					return
				default:
					if page.IsClosed() {
						return
					}
					if checkLoggedInWithTimeout(page, 1*time.Second) {
						fmt.Println("\nLogin detected automatically!")
						done <- true
						return
					}
					time.Sleep(1500 * time.Millisecond)
				}
			}
		}()

		// Goroutine to wait for manual Enter key
		go func() {
			var reader *bufio.Reader
			if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
				defer tty.Close()
				reader = bufio.NewReader(tty)
			} else {
				reader = bufio.NewReader(os.Stdin)
			}
			_, _ = reader.ReadString('\n')
			done <- true
		}()

		<-done
	}

	// Now we are logged in (or forced to continue)
	// Check if VM is active
	fmt.Println("Checking target VM status...")
	hasActiveVM, _ := page.Evaluate(`(() => {
		const activeInfo = document.getElementById('active-machine-info') || 
		                   document.getElementById('active-machines-information-header') ||
		                   document.querySelector('[id^="active-machines-information"]') ||
		                   document.querySelector('[class*="active-machines-information"]');
		if (activeInfo) {
			const text = activeInfo.innerText.toLowerCase();
			if (text.includes("terminate") || text.includes("add 1 hour") || /\b10\.\d+\.\d+\.\d+\b/.test(text)) {
				return true;
			}
		}
		const btns = Array.from(document.querySelectorAll('button'));
		const hasTerminate = btns.some(b => b.innerText.toLowerCase().includes("terminate"));
		const hasAddHour = btns.some(b => b.innerText.toLowerCase().includes("add 1 hour"));
		return hasTerminate || hasAddHour;
	})()`)

	if !hasActiveVM.(bool) {
		fmt.Println("Target VM is not running. Nothing to terminate.")
		_ = hosts.RemoveEntry(roomName)
		fmt.Printf("Saving updated session state to %s...\n", browser.SESSION_FILE)
		_ = ctx.SaveState(browser.SESSION_FILE)
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
			fmt.Println("Successfully terminated VM!")
			_ = hosts.RemoveEntry(roomName)
			fmt.Printf("Saving updated session state to %s...\n", browser.SESSION_FILE)
			_ = ctx.SaveState(browser.SESSION_FILE)
			browser.CleanScreenshots()
			time.Sleep(3 * time.Second)
			return
		}
	}

	fmt.Println("Warning: Failed to click 'Terminate' button automatically.")
	fmt.Printf("Saving updated session state to %s...\n", browser.SESSION_FILE)
	_ = ctx.SaveState(browser.SESSION_FILE)
	browser.CleanScreenshots()
	time.Sleep(1 * time.Second)
}

func startLoopDaemon(roomName string) {
	// Let's check if there is a running loop daemon PID file
	pidFile := filepath.Join(os.TempDir(), fmt.Sprintf("init-vm-loop-%s.pid", roomName))
	if data, err := os.ReadFile(pidFile); err == nil {
		var pid int
		if _, err := fmt.Sscanf(string(data), "%d", &pid); err == nil {
			// Check if process is still running
			if process, err := os.FindProcess(pid); err == nil {
				// On Unix/macOS, sending signal 0 checks if process exists
				if err := process.Signal(syscall.Signal(0)); err == nil {
					fmt.Printf("Loop daemon already running for room %s (PID %d)\n", roomName, pid)
					return
				}
			}
		}
	}

	exe, err := os.Executable()
	if err != nil {
		fmt.Printf("Warning: could not get executable path to start loop daemon: %v\n", err)
		return
	}

	cmd := exec.Command(exe, roomName, "daemon")
	projectRoot := getProjectRoot()
	cmd.Dir = projectRoot
	
	// Create a log file inside the room directory (or in /tmp if room directory doesn't exist)
	roomDir := filepath.Join(projectRoot, roomName)
	logDir := roomDir
	if fi, err := os.Stat(roomDir); err != nil || !fi.IsDir() {
		logDir = os.TempDir()
	}
	logFile, err := os.OpenFile(filepath.Join(logDir, "loop.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	} else {
		cmd.Stdout = nil
		cmd.Stderr = nil
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}

	if err := cmd.Start(); err != nil {
		fmt.Printf("Warning: failed to start loop daemon in background: %v\n", err)
	} else {
		fmt.Printf("Started loop daemon in background (PID %d). Log file: %s\n", cmd.Process.Pid, filepath.Join(logDir, "loop.log"))
		_ = os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0644)
	}
}

func stopLoopDaemon(roomName string) {
	pidFile := filepath.Join(os.TempDir(), fmt.Sprintf("init-vm-loop-%s.pid", roomName))
	if data, err := os.ReadFile(pidFile); err == nil {
		var pid int
		if _, err := fmt.Sscanf(string(data), "%d", &pid); err == nil {
			if process, err := os.FindProcess(pid); err == nil {
				fmt.Printf("Stopping loop daemon (PID %d)...", pid)
				_ = process.Signal(syscall.SIGTERM)
				
				// Wait a bit for the process to exit
				for i := 0; i < 20; i++ {
					if err := process.Signal(syscall.Signal(0)); err != nil {
						break
					}
					time.Sleep(100 * time.Millisecond)
				}
				if err := process.Signal(syscall.Signal(0)); err == nil {
					_ = process.Kill()
				}
				fmt.Println(" Loop daemon stopped.")
			}
		}
		_ = os.Remove(pidFile)
	}
}

// LoopExtend runs Extend every 30 minutes until terminated with Ctrl+C
func LoopExtend(roomName string) {
	fmt.Printf("Starting auto-extend daemon for room '%s' (running every 30 minutes)...\n", roomName)
	fmt.Println("Press [Ctrl+C] to stop this loop.")

	// Write PID file for the loop
	pidFile := filepath.Join(os.TempDir(), fmt.Sprintf("init-vm-loop-%s.pid", roomName))
	_ = os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)

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
			_ = os.Remove(pidFile)
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
		const el = document.getElementById('active-machine-info') || 
		           document.getElementById('active-machines-information-header') || 
		           document.querySelector('[id^="active-machines-information"]') ||
		           document.querySelector('[class*="active-machines-information"]') ||
		           document.body;
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
			const el = document.getElementById('active-machine-info') || 
			           document.getElementById('active-machines-information-header') || 
			           document.querySelector('[id^="active-machines-information"]') ||
			           document.querySelector('[class*="active-machines-information"]') ||
			           document.body;
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

func findScript(name string) (string, error) {
	// Search in current dir, parent dir, executable dir, and executable parent dir
	searchDirs := []string{".", ".."}

	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		searchDirs = append(searchDirs, exeDir, filepath.Dir(exeDir))
	}

	for _, dir := range searchDirs {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			absPath, err := filepath.Abs(p)
			if err != nil {
				return "", err
			}
			return absPath, nil
		}
	}
	return "", fmt.Errorf("%s not found in search paths", name)
}

func getProjectRoot() string {
	if vpnPath, err := findScript("vpn.sh"); err == nil {
		return filepath.Dir(vpnPath)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func runVPN(action string) error {
	vpnPath, err := findScript("vpn.sh")
	if err != nil {
		return err
	}

	cmd := exec.Command(vpnPath, action)
	cmd.Dir = filepath.Dir(vpnPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runScan(roomName string, ip string) {
	projectRoot := getProjectRoot()
	roomDir := filepath.Join(projectRoot, roomName)
	roomDirExists := false
	if fi, err := os.Stat(roomDir); err == nil && fi.IsDir() {
		roomDirExists = true
	}

	nmapPath := filepath.Join(roomDir, roomName+".nmap")
	nmapExists := false
	if _, err := os.Stat(nmapPath); err == nil {
		nmapExists = true
	}

	if roomDirExists && nmapExists {
		fmt.Printf("Directory %s already exists and nmap scan %s is already done. Skipping scan.\n", roomDir, nmapPath)
		return
	}

	scanPath, err := findScript("scan.sh")
	if err != nil {
		fmt.Printf("Warning: %v\n", err)
		return
	}

	fmt.Printf("Starting scan.sh with IP %s...\n", ip)

	if err := os.MkdirAll(roomDir, 0755); err != nil {
		fmt.Printf("Warning: failed to create room directory %s: %v\n", roomDir, err)
		return
	}

	// cd into the new VM directory
	if err := os.Chdir(roomDir); err != nil {
		fmt.Printf("Warning: failed to cd into room directory %s: %v\n", roomDir, err)
		return
	}

	cmd := exec.Command(scanPath, "-n", roomName, ip)
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

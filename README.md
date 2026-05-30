# TryHackMe VM Automation Suite

This workspace contains the `init-vm` CLI tool and helper scripts designed to streamline and automate the setup, connection, and scanning phases of TryHackMe rooms.

## Overview

The `init-vm` tool automates the interaction with the TryHackMe website using a headless/headful browser backend. Once started, it automatically manages the life cycle of target VMs, local OpenVPN connections, hosts file mappings, and scanning tools.

### Core Features

1. **Automatic VM Deployment**: Navigates to the specified TryHackMe room, logs in using session files or prompts for manual login if the session is expired, and deploys the VM.
2. **Directory Isolation**: Creates a dedicated subdirectory for each room to organize scanning results and CTF notes.
3. **Automatic OpenVPN Management**: Interacts with `vpn.sh` to start/stop the OpenVPN client automatically.
4. **IP & DNS Resolution**: Polls the room page until the VM's IP address is assigned, displays it, and writes an entry in `/etc/hosts` mapping `<room_name>.thm` to the IP.
5. **Autostart Scan**: Invokes `scan.sh` to run `nmap` against the target VM and saves the results directly inside the room directory.
6. **Daemon Life-cycle Extender**: Runs in the background and extends the VM lease every 30 minutes to prevent the machine from timing out during long sessions.
7. **Clean Termination**: Shuts down the VM on TryHackMe, cleans up `/etc/hosts` entries, and stops the local OpenVPN client.

---

## File Structure

```text
/Users/j0rdan0/ctf/tryhackme/
├── init-vm/            # Go source code for the CLI tool
│   ├── cmd/            # Command-line interface definitions (Cobra)
│   ├── pkg/
│   │   ├── browser/    # Browser automation controls
│   │   ├── hosts/      # Local /etc/hosts modification utility
│   │   └── vm/         # VM orchestration logic
│   └── main.go         # Go entry point
├── vpn.sh              # OpenVPN helper script (start/stop/restart)
├── scan.sh             # Nmap port scanning script
└── *.ovpn              # TryHackMe OpenVPN configuration file(s)
```

---

## Usage

### 1. Build the program
To compile the `init-vm` tool, navigate to the `init-vm` directory and build:
```bash
cd init-vm
go build -o init-vm
```

### 2. Start a Room VM
Deploys the room VM, creates the room directory, starts the VPN, registers the hostname, and triggers the port scan:
```bash
./init-vm/init-vm cheesectfv10 start
```
*Alternatively, the program supports the standard command-first syntax:*
```bash
./init-vm/init-vm start cheesectfv10
```

### 3. Extend VM Lease
Adds 1 hour to the active machine's duration:
```bash
./init-vm/init-vm cheesectfv10 extend
# OR
./init-vm/init-vm extend cheesectfv10
```

### 4. Keep-Alive Daemon (Auto-Extend)
Keeps the VM running continuously by extending it automatically every 30 minutes:
```bash
./init-vm/init-vm cheesectfv10 loop
# OR
./init-vm/init-vm loop cheesectfv10
```

### 5. Terminate VM
Stops the local OpenVPN connection, terminates the TryHackMe VM, and cleans up the `/etc/hosts` entry:
```bash
./init-vm/init-vm cheesectfv10 terminate
# OR
./init-vm/init-vm terminate cheesectfv10
```

---

## Script Integrations

### VPN Script (`vpn.sh`)
Manages OpenVPN client processes.
- **Start**: `sudo vpn.sh start`
  - Scans the root directory for `.ovpn` or `.conf` configuration files.
  - Launches OpenVPN as a daemon and saves the process PID to `/tmp/openvpn_tryhackme.pid`.
- **Stop**: `sudo vpn.sh stop`
  - Safely stops the active OpenVPN daemon process.

### Scan Script (`scan.sh`)
Automates host discovery and vulnerability scanning using `nmap`.
- Runs a fast service/script scan by default:
  ```bash
  sudo ./scan.sh <target-ip>
  ```
- Optional flags:
  - `-a` : Scans all 65,535 ports (`-p-`).
  - `-u` : Conducts a UDP port scan (`-sU`).

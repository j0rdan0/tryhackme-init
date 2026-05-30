#!/bin/bash

# Define the PID file location
PID_FILE="/tmp/openvpn_tryhackme.pid"

# Find the first .ovpn or .conf file in the current working directory
CONFIG_FILE=""
for f in *.ovpn *.conf; do
    if [ -f "$f" ]; then
        CONFIG_FILE="$f"
        break
    fi
done

stop_vpn() {
    if [ -f "$PID_FILE" ]; then
        PID=$(cat "$PID_FILE")
        if ps -p "$PID" > /dev/null 2>&1; then
            echo "Stopping OpenVPN (PID $PID)..."
            sudo kill "$PID"
            
            # Wait up to 10 seconds for the process to stop
            for i in {1..20}; do
                if ! ps -p "$PID" > /dev/null 2>&1; then
                    break
                fi
                sleep 0.5
            done
            
            if ps -p "$PID" > /dev/null 2>&1; then
                echo "Warning: OpenVPN (PID $PID) did not stop. Forcing shutdown..."
                sudo kill -9 "$PID"
            fi
            echo "OpenVPN stopped."
        else
            echo "OpenVPN PID file found, but process $PID is not running."
        fi
        rm -f "$PID_FILE"
    else
        # Fallback: check if any openvpn process is running and stop it
        if pgrep openvpn > /dev/null 2>&1; then
            echo "No PID file found, but openvpn processes detected. Stopping them..."
            sudo killall openvpn
        else
            echo "OpenVPN is not running."
        fi
    fi
}

start_vpn() {
    if [ -z "$CONFIG_FILE" ]; then
        echo "Error: No .ovpn or .conf configuration file found in the current directory."
        exit 1
    fi

    # Check if already running
    if [ -f "$PID_FILE" ]; then
        PID=$(cat "$PID_FILE")
        if ps -p "$PID" > /dev/null 2>&1; then
            echo "OpenVPN is already running (PID $PID)."
            exit 0
        else
            # Process is dead, clean up PID file
            rm -f "$PID_FILE"
        fi
    fi

    echo "Starting OpenVPN with config: $CONFIG_FILE"
    # Run openvpn as daemon and write PID
    sudo openvpn --config "$CONFIG_FILE" --daemon --writepid "$PID_FILE"
    
    # Wait a bit to check if it started successfully
    sleep 1.5
    if [ -f "$PID_FILE" ]; then
        PID=$(cat "$PID_FILE")
        if ps -p "$PID" > /dev/null 2>&1; then
            echo "OpenVPN started successfully (PID $PID)."
        else
            echo "Error: OpenVPN started but terminated. Check system logs / openvpn output."
            rm -f "$PID_FILE"
            exit 1
        fi
    else
        echo "Error: Failed to start OpenVPN (PID file was not created)."
        exit 1
    fi
}

case "$1" in
    start)
        start_vpn
        ;;
    stop)
        stop_vpn
        ;;
    restart)
        stop_vpn
        sleep 1
        start_vpn
        ;;
    *)
        echo "Usage: $0 {start|stop|restart}"
        exit 1
        ;;
esac

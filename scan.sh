#!/bin/bash

# Default values
ALL_PORTS=false
UDP_SCAN=false

# Usage function
usage() {
    echo "Usage: $0 [-a] [-u] [-n <name>] <hostname/IP>"
    echo "  -a   Scan all ports (-p-), outputting to <name>-all.nmap"
    echo "  -u   Run a UDP scan (-sU), outputting to <name>-udp.nmap"
    echo "  -n   Specify custom name for the output file instead of hostname/IP"
    exit 1
}

# Parse options
NAME=""
while getopts "aun:" opt; do
    case ${opt} in
        a )
            ALL_PORTS=true
            ;;
        u )
            UDP_SCAN=true
            ;;
        n )
            NAME="${OPTARG}"
            ;;
        \? )
            usage
            ;;
    esac
done
shift $((OPTIND -1))

# The remaining argument is the host
if [ -z "$1" ]; then
    echo "Error: Hostname or IP is required."
    usage
fi

HOST="$1"

# Wait for the host to become reachable (VPN route to establish)
echo "Waiting for host ${HOST} to become reachable..."
ping_success=false
for i in {1..30}; do
    if [ "$(uname)" = "Darwin" ]; then
        if ping -c 1 -t 2 "${HOST}" >/dev/null 2>&1; then
            ping_success=true
            break
        fi
    else
        if ping -c 1 -W 2 "${HOST}" >/dev/null 2>&1; then
            ping_success=true
            break
        fi
    fi
    sleep 1
done

if [ "$ping_success" = true ]; then
    echo "Host ${HOST} is reachable via ping."
    echo "Waiting for services to start (checking common ports)..."
    
    COMMON_PORTS=(22 80 443 445 3389 8080 21 23 25 139)
    services_ready=false
    
    for attempt in {1..20}; do
        for port in "${COMMON_PORTS[@]}"; do
            if command -v nc >/dev/null 2>&1; then
                if nc -z -w 1 "${HOST}" "$port" >/dev/null 2>&1; then
                    echo "Detected open port ${port}. Services are starting up!"
                    services_ready=true
                    break 2
                fi
            else
                if (echo > "/dev/tcp/${HOST}/${port}") >/dev/null 2>&1; then
                    echo "Detected open port ${port}. Services are starting up!"
                    services_ready=true
                    break 2
                fi
            fi
        done
        sleep 1.5
    done
    
    if [ "$services_ready" = false ]; then
        echo "No common ports responded within timeout. Proceeding with scan anyway..."
    else
        # Give a small 3-second buffer for other services to fully bind
        sleep 3
    fi
else
    echo "Warning: Host ${HOST} did not respond to ping. Proceeding with scan anyway..."
fi

# Construct command and filename
NMAP_FLAGS="-Pn -sV -T4 -sC"
SUFFIX=""

# On macOS, TCP SYN scan (-sS) via raw sockets often fails over virtual tun/utun interfaces.
# Using TCP connect scan (-sT) is much more reliable and standard.
if [ "$(uname)" = "Darwin" ] && [ "$UDP_SCAN" = false ]; then
    NMAP_FLAGS="${NMAP_FLAGS} -sT"
fi

if [ "$ALL_PORTS" = true ]; then
    NMAP_FLAGS="${NMAP_FLAGS} -p-"
    SUFFIX="${SUFFIX}-all"
fi

if [ "$UDP_SCAN" = true ]; then
    NMAP_FLAGS="${NMAP_FLAGS} -sU"
    SUFFIX="${SUFFIX}-udp"
fi

if [ -z "$NAME" ]; then
    NAME="${HOST}"
fi

OUTPUT_FILE="${NAME}${SUFFIX}.nmap"

echo "Executing: sudo nmap ${NMAP_FLAGS} -oN ${OUTPUT_FILE} ${HOST}"
sudo nmap ${NMAP_FLAGS} -oN "${OUTPUT_FILE}" "${HOST}"

# Check if nmap scan was successful and output file exists
if [ -f "${OUTPUT_FILE}" ]; then
    echo "Parsing nmap results to check for web servers..."
    
    # Determine the target host name/IP for feroxbuster
    if [[ "$NAME" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        TARGET_HOST="$NAME"
    else
        TARGET_HOST="${NAME}.thm"
    fi

    # Read nmap output file line by line
    while read -r line; do
        # Match lines like "80/tcp open http ..."
        if [[ "$line" =~ ^([0-9]+)/(tcp|udp)[[:space:]]+open[[:space:]]+([^[:space:]]+) ]]; then
            port="${BASH_REMATCH[1]}"
            proto="${BASH_REMATCH[2]}"
            service="${BASH_REMATCH[3]}"
            
            is_web=false
            scheme="http"
            
            if [ "$port" = "80" ]; then
                is_web=true
                scheme="http"
            elif [ "$port" = "443" ]; then
                is_web=true
                scheme="https"
            elif [[ "$service" =~ http ]]; then
                is_web=true
                if [[ "$service" =~ https ]] || [[ "$service" =~ ssl/http ]]; then
                    scheme="https"
                else
                    scheme="http"
                fi
            fi
            
            if [ "$is_web" = true ]; then
                if [ "$port" = "80" ] && [ "$scheme" = "http" ]; then
                    url="http://${TARGET_HOST}"
                elif [ "$port" = "443" ] && [ "$scheme" = "https" ]; then
                    url="https://${TARGET_HOST}"
                else
                    url="${scheme}://${TARGET_HOST}:${port}"
                fi
                
                if [ "$port" = "80" ] || [ "$port" = "443" ]; then
                    output_file="output.ferox"
                else
                    output_file="output-${port}.ferox"
                fi
                
                echo "[+] Web server detected on port ${port} (${url})."
                echo "[+] Starting feroxbuster scan in the background..."
                echo "    Command: feroxbuster -k --url ${url} -w ~/pentest-tools/SecLists/Discovery/Web-Content/raft-medium-directories.txt -x php,txt --scan-dir-listings --output ${output_file}"
                
                # Run feroxbuster in the background
                nohup feroxbuster -k --url "${url}" \
                    -w "${HOME}/pentest-tools/SecLists/Discovery/Web-Content/raft-medium-directories.txt" \
                    -x php,txt \
                    --scan-dir-listings \
                    --output "${output_file}" > "feroxbuster-${port}.log" 2>&1 &
            fi
        fi
    done < "${OUTPUT_FILE}"
fi


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

# Construct command and filename
NMAP_FLAGS="-Pn -sV -T4 -sC"
SUFFIX=""

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
    fi < "${OUTPUT_FILE}"
fi


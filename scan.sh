#!/bin/bash

# Default values
ALL_PORTS=false
UDP_SCAN=false

# Usage function
usage() {
    echo "Usage: $0 [-a] [-u] <hostname/IP>"
    echo "  -a   Scan all ports (-p-), outputting to <host>-all.nmap"
    echo "  -u   Run a UDP scan (-sU), outputting to <host>-udp.nmap"
    exit 1
}

# Parse options
while getopts "au" opt; do
    case ${opt} in
        a )
            ALL_PORTS=true
            ;;
        u )
            UDP_SCAN=true
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
NMAP_FLAGS="-sV -T4 -sC"
SUFFIX=""

if [ "$ALL_PORTS" = true ]; then
    NMAP_FLAGS="${NMAP_FLAGS} -p-"
    SUFFIX="${SUFFIX}-all"
fi

if [ "$UDP_SCAN" = true ]; then
    NMAP_FLAGS="${NMAP_FLAGS} -sU"
    SUFFIX="${SUFFIX}-udp"
fi

OUTPUT_FILE="${HOST}${SUFFIX}.nmap"

echo "Executing: sudo nmap ${NMAP_FLAGS} -oN ${OUTPUT_FILE} ${HOST}"
sudo nmap ${NMAP_FLAGS} -oN "${OUTPUT_FILE}" "${HOST}"

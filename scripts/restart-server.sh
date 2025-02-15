#!/bin/bash
# restart-server.sh
# This script stops any running "server" process and restarts it.
# It assumes the server binary is located in the user's home directory as "server".

# Kill any running server process; ignore errors if none are running.
pkill -f server || true

# Wait for a moment to ensure the old process has terminated.
sleep 1

# Restart the server in the background.
# The output is redirected to server.log.
nohup ~/server > server.log 2>&1 &

echo "Server restarted."

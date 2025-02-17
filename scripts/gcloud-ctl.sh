#!/bin/bash
# gcloud-ctl.sh
# This script creates/destroys two Google Cloud VMs (one for the server and one for the client),
# deploys the built binaries to each VM, and can also "run" the deployment.
#
# In "run" mode, the script SSHs into the server VM to stop any running server and restart it,
# then SSHs into the client VM to run a provided command.
#
# Usage:
#   ./gcloud-ctl.sh create
#   ./gcloud-ctl.sh destroy
#   ./gcloud-ctl.sh run "<command to run on client>"
#
# Example for run mode:
#   ./gcloud-ctl.sh run "./client -host=10.128.0.2:50051 -mode=bidirectional -interval=1 -transactions=1000 -client-id=myClient -x 3 -y 1 -operation=isprime -latency-gt=5 -workers=3 -jwt-gen=once"

set -e

# --- Configuration Variables ---
PROJECT="anthony-lab"                   # Your GCP project ID.
ZONE="us-central1-a"                    # Your desired zone.
MACHINE_TYPE="e2-medium"                # Machine type.
IMAGE_FAMILY="debian-11"                # OS image family.
IMAGE_PROJECT="debian-cloud"            # Project for the OS image.
SERVER_NAME="grpc-server"
CLIENT_NAME="grpc-client"

# Directory containing the built binaries (client and server).
BIN_DIR="bin"

# --- Usage Function ---
function usage() {
    echo "Usage: $0 [create|destroy|run <command>]"
    exit 1
}

if [ "$#" -lt 1 ]; then
    usage
fi

ACTION="$1"
shift

# --- Helper: Check if a VM exists ---
function vmExists() {
    local vmName=$1
    if gcloud compute instances list --filter="name=(${vmName})" --zones="$ZONE" --format="value(name)" | grep -q "^${vmName}$"; then
        return 0
    else
        return 1
    fi
}

if [ "$ACTION" == "create" ]; then
    echo "Setting up VM instances in project '$PROJECT', zone '$ZONE'..."

    # Create the server instance if it doesn't exist.
    if vmExists "$SERVER_NAME"; then
        echo "Server instance '$SERVER_NAME' already exists."
    else
        echo "Creating server instance: $SERVER_NAME"
        gcloud compute instances create $SERVER_NAME \
            --project="$PROJECT" \
            --zone="$ZONE" \
            --machine-type="$MACHINE_TYPE" \
            --image-family="$IMAGE_FAMILY" \
            --image-project="$IMAGE_PROJECT" \
            --tags=grpc-server
    fi

    # Create the client instance if it doesn't exist.
    if vmExists "$CLIENT_NAME"; then
        echo "Client instance '$CLIENT_NAME' already exists."
    else
        echo "Creating client instance: $CLIENT_NAME"
        gcloud compute instances create $CLIENT_NAME \
            --project="$PROJECT" \
            --zone="$ZONE" \
            --machine-type="$MACHINE_TYPE" \
            --image-family="$IMAGE_FAMILY" \
            --image-project="$IMAGE_PROJECT" \
            --tags=grpc-client
    fi

    echo "VM instances are ready."

    # --- Retrieve Internal IPs ---
    SERVER_IP=$(gcloud compute instances describe "$SERVER_NAME" --zone="$ZONE" --format='get(networkInterfaces[0].networkIP)')
    CLIENT_IP=$(gcloud compute instances describe "$CLIENT_NAME" --zone="$ZONE" --format='get(networkInterfaces[0].networkIP)')
    echo "Server internal IP: $SERVER_IP"
    echo "Client internal IP: $CLIENT_IP"

    # --- Deploy Binaries ---
    echo "Deploying built binaries to VMs..."

    if [ ! -d "$BIN_DIR" ]; then
        echo "Error: Binaries directory '$BIN_DIR' not found. Please build your binaries first."
        exit 1
    fi

    for vm in "$SERVER_NAME" "$CLIENT_NAME"; do
        echo "Deploying to $vm..."
        gcloud compute scp --quiet --project="$PROJECT" --zone="$ZONE" "$BIN_DIR/client" "$vm:~/client"
        gcloud compute scp --quiet --project="$PROJECT" --zone="$ZONE" "$BIN_DIR/server" "$vm:~/server"
    done

    echo "Deployment complete:"
    echo "  Server: $SERVER_NAME ($SERVER_IP)"
    echo "  Client: $CLIENT_NAME ($CLIENT_IP)"

    echo "Next Steps:"
    echo "  Interact with the client and server on the gcloud VMs. Recommended to open each in a separate terminal window"
    echo "  gcloud compute ssh grpc-client --zone=$ZONE --project=$PROJECT"
    echo "  gcloud compute ssh grpc-server --zone=$ZONE --project=$PROJECT"

elif [ "$ACTION" == "destroy" ]; then
    echo "Destroying VM instances in project '$PROJECT', zone '$ZONE'..."
    gcloud compute instances delete $SERVER_NAME $CLIENT_NAME --zone="$ZONE" --quiet
    echo "VM instances destroyed."
else
    usage
fi

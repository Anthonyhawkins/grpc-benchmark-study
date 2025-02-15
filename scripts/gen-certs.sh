#!/bin/bash
# gen-certs.sh - Generate CA, server, and client certificates for mutual TLS with IP SAN.
#
# This script creates a "certs" directory (if it doesn't exist) and generates:
#   ca.key       - Private key for our Certificate Authority (CA)
#   ca.crt       - Self-signed CA certificate
#   server.key   - Private key for the server
#   server.csr   - Certificate signing request for the server
#   server.crt   - Server certificate signed by our CA (with IP SAN for 127.0.0.1 and DNS:localhost)
#   client.key   - Private key for the client
#   client.csr   - Certificate signing request for the client
#   client.crt   - Client certificate signed by our CA
#
# Exit immediately if a command exits with a non-zero status.
set -e

# Variables
CERTS_DIR="certs"
DAYS_VALID=365

CA_KEY="ca.key"
CA_CERT="ca.crt"

SERVER_KEY="server.key"
SERVER_CSR="server.csr"
SERVER_CERT="server.crt"

CLIENT_KEY="client.key"
CLIENT_CSR="client.csr"
CLIENT_CERT="client.crt"

# Create the certs directory if it doesn't exist.
mkdir -p "${CERTS_DIR}"
cd "${CERTS_DIR}"

echo "Generating CA private key..."
openssl genrsa -out "${CA_KEY}" 4096

echo "Generating self-signed CA certificate..."
openssl req -x509 -new -nodes -key "${CA_KEY}" -sha256 -days ${DAYS_VALID} \
    -out "${CA_CERT}" \
    -subj "/C=US/ST=State/L=City/O=Organization/OU=OrgUnit/CN=MyCA"

echo "Generating Server private key..."
openssl genrsa -out "${SERVER_KEY}" 2048

echo "Generating Server CSR..."
openssl req -new -key "${SERVER_KEY}" -out "${SERVER_CSR}" \
    -subj "/C=US/ST=State/L=City/O=Organization/OU=Server/CN=localhost"

# Create a temporary file with the subjectAltName configuration.
cat > server.ext <<EOF
subjectAltName = IP:127.0.0.1, DNS:localhost
EOF

echo "Signing Server certificate with our CA (including IP SAN)..."
openssl x509 -req -in "${SERVER_CSR}" -CA "${CA_CERT}" -CAkey "${CA_KEY}" \
    -CAcreateserial -out "${SERVER_CERT}" -days ${DAYS_VALID} -sha256 \
    -extfile server.ext

echo "Generating Client private key..."
openssl genrsa -out "${CLIENT_KEY}" 2048

echo "Generating Client CSR..."
openssl req -new -key "${CLIENT_KEY}" -out "${CLIENT_CSR}" \
    -subj "/C=US/ST=State/L=City/O=Organization/OU=Client/CN=client"

echo "Signing Client certificate with our CA..."
openssl x509 -req -in "${CLIENT_CSR}" -CA "${CA_CERT}" -CAkey "${CA_KEY}" \
    -CAcreateserial -out "${CLIENT_CERT}" -days ${DAYS_VALID} -sha256

echo "Certificates and keys generated in the '${CERTS_DIR}' directory:"
ls -l

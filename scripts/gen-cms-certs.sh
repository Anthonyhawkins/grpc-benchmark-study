#!/bin/bash
# gen-pkcs7-certs.sh - Generate CA and signer certificates/keys for PKCS#7 signing.
#
# This script will create a "pkcs7-certs" directory (if it doesn't exist)
# and generate the following files inside it:
#
#   ca.key       - Private key for the Certificate Authority (CA)
#   ca.crt       - Self-signed CA certificate
#   signer.key   - Private key for the signing certificate
#   signer.csr   - Certificate signing request for the signer
#   signer.crt   - Signer certificate signed by the CA
#
# Adjust the subject details as needed.
#
# Exit immediately if any command fails.
set -e

# Variables
CERTS_DIR="pkcs7-certs"
DAYS_VALID=365

CA_KEY="ca.key"
CA_CERT="ca.crt"

SIGNER_KEY="signer.key"
SIGNER_CSR="signer.csr"
SIGNER_CERT="signer.crt"

# Create the output directory if it doesn't exist.
mkdir -p "${CERTS_DIR}"
cd "${CERTS_DIR}"

echo "Generating CA private key..."
openssl genrsa -out "${CA_KEY}" 4096

echo "Generating self-signed CA certificate..."
openssl req -x509 -new -nodes -key "${CA_KEY}" -sha256 -days ${DAYS_VALID} \
    -out "${CA_CERT}" \
    -subj "/C=US/ST=State/L=City/O=Organization/OU=CA/CN=MyCA"

echo "Generating Signer private key..."
openssl genrsa -out "${SIGNER_KEY}" 2048

echo "Generating Signer CSR..."
openssl req -new -key "${SIGNER_KEY}" -out "${SIGNER_CSR}" \
    -subj "/C=US/ST=State/L=City/O=Organization/OU=Signer/CN=Signer"

echo "Signing Signer certificate with our CA..."
openssl x509 -req -in "${SIGNER_CSR}" -CA "${CA_CERT}" -CAkey "${CA_KEY}" \
    -CAcreateserial -out "${SIGNER_CERT}" -days ${DAYS_VALID} -sha256

echo "Certificates and keys generated in the '${CERTS_DIR}' directory:"
ls -l

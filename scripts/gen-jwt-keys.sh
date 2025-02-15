#!/bin/bash
# gen-jwt-keys.sh - Generate RSA keys for JWT signing and verification.
#
# This script generates:
#   jwt.key  - The RSA private key (used by the client to sign JWT tokens)
#   jwt.pub  - The RSA public key (used by the server to verify JWT tokens)
#
# Exit immediately if a command fails.
set -e

# Key length (2048 bits is standard; adjust if needed)
KEY_LENGTH=2048

echo "Generating RSA private key for JWT signing..."
openssl genrsa -out jwt.key ${KEY_LENGTH}

echo "Deriving RSA public key for JWT verification..."
openssl rsa -in jwt.key -pubout -out jwt.pub

echo "JWT keys generated:"
ls -l jwt.key jwt.pub

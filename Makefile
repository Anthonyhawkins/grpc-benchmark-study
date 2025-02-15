# Makefile for building client and server binaries for Google Cloud VM deployment.
# This Makefile assumes the following project structure:
#
#   cmd/client/client.go
#   cmd/server/server.go
#
# The binaries will be placed in the "bin" directory.

APP_NAME_CLIENT = client
APP_NAME_SERVER = server
BUILD_DIR = bin

# Target OS and architecture for deployment.
GOOS ?= linux
GOARCH ?= amd64
CGO_ENABLED ?= 0

# Directories where the client and server source code is located.
CLIENT_DIR = cmd/client
SERVER_DIR = cmd/server

.PHONY: all client server clean

all: client server

client:
	mkdir -p $(BUILD_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) go build -o $(BUILD_DIR)/$(APP_NAME_CLIENT) $(CLIENT_DIR)/client.go

server:
	mkdir -p $(BUILD_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) go build -o $(BUILD_DIR)/$(APP_NAME_SERVER) $(SERVER_DIR)/server.go

clean:
	rm -rf $(BUILD_DIR)

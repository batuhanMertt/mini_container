BINARY_NAME=mini-container
ROOTFS_DIR=assets/rootfs

.PHONY: all build setup clean run

all: build

setup:
	rm -rf $(BINARY_NAME) assets/
	@echo "Alpine Linux rootfs indiriliyor..."
	mkdir -p $(ROOTFS_DIR)
	curl -sL https://dl-cdn.alpinelinux.org/alpine/v3.20/releases/x86_64/alpine-minirootfs-3.20.0-x86_64.tar.gz | tar -xz -C $(ROOTFS_DIR)
	@echo "Rootfs hazir."

build:
	go build -o $(BINARY_NAME) main.go

run: build
	sudo ./$(BINARY_NAME) run /bin/sh

clean:

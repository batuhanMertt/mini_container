BINARY_NAME=mini-container
LAUNCHER_NAME=mini-container-launcher
ROOTFS_DIR=assets/rootfs
ALPINE_BRANCH=v3.20
ALPINE_VERSION=3.20.0
ALPINE_ARCH=x86_64
VERSION?=dev
LDFLAGS=-s -w -X main.version=$(VERSION)

.PHONY: all build launcher setup clean run dist check

all: build

## setup: download the Alpine minirootfs the container is chrooted into
setup:
	rm -rf $(BINARY_NAME) assets/
	@echo "Downloading the Alpine Linux rootfs..."
	mkdir -p $(ROOTFS_DIR)
	curl -sL https://dl-cdn.alpinelinux.org/alpine/$(ALPINE_BRANCH)/releases/$(ALPINE_ARCH)/alpine-minirootfs-$(ALPINE_VERSION)-$(ALPINE_ARCH).tar.gz | tar -xz -C $(ROOTFS_DIR)
	@echo "Rootfs ready."

## build: build the Linux runtime for this machine
build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) ./cmd/mini-container

## launcher: build the host-side launcher (Windows/macOS/Linux)
launcher:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(LAUNCHER_NAME) ./cmd/mini-container-launcher

## run: build and start a shell inside the container (Linux only)
run: build
	sudo ./$(BINARY_NAME) run /bin/sh

## check: what CI enforces - formatting, vet, and every release target
check:
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed:"; gofmt -l .; exit 1)
	go vet ./...
	@for t in linux/amd64 linux/arm64 windows/amd64 windows/arm64 darwin/amd64 darwin/arm64; do \
		for pkg in ./cmd/mini-container ./cmd/mini-container-launcher; do \
			echo "==> $$t $$pkg"; \
			GOOS=$${t%/*} GOARCH=$${t#*/} go build -o /dev/null $$pkg || exit 1; \
		done; \
	done

## dist: build every published release asset into dist/
dist:
	rm -rf dist && mkdir -p dist
	@for arch in amd64 arm64; do \
		GOOS=linux GOARCH=$$arch CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" \
			-o dist/$(BINARY_NAME)-runtime_linux_$$arch ./cmd/mini-container || exit 1; \
	done
	@for t in windows/amd64 windows/arm64 darwin/amd64 darwin/arm64 linux/amd64 linux/arm64; do \
		os=$${t%/*}; arch=$${t#*/}; out=dist/$(BINARY_NAME)_$${os}_$${arch}; \
		[ "$$os" = windows ] && out=$$out.exe; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" \
			-o $$out ./cmd/mini-container-launcher || exit 1; \
	done
	cd dist && sha256sum * > checksums.txt
	@ls -la dist

clean:
	rm -rf $(BINARY_NAME) $(LAUNCHER_NAME) dist/ assets/

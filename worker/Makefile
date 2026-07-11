SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c

LOCAL_DIR ?= .local
BIN_DIR ?= $(LOCAL_DIR)/bin
VM_DIR ?= $(LOCAL_DIR)/vm

FIRECRACKER_RELEASES_URL ?= https://github.com/firecracker-microvm/firecracker/releases
FIRECRACKER_VERSION ?= latest
FIRECRACKER_ARCH ?= x86_64
FIRECRACKER_BIN := $(BIN_DIR)/firecracker
AGENT_BIN := $(BIN_DIR)/agent

FIRECRACKER_CI_S3 ?= https://s3.amazonaws.com/spec.ccfc.min
ROOTFS_SIZE ?= 1G

.PHONY: help setup dirs build-agent firecracker vm-assets check-linux check-tools check-vm-tools kvm-check

help:
	@printf '%s\n' \
		'Targets:' \
		'  make setup       Build the worker agent and Firecracker assets' \
		'  make build-agent  Build .local/bin/agent' \
		'  make firecracker Download Firecracker into .local/bin' \
		'  make vm-assets   Build the agent rootfs in .local/vm (requires sudo)' \
		'  make kvm-check   Verify /dev/kvm access and show suggested fixes' \
		'' 

setup: check-linux kvm-check check-tools check-vm-tools dirs build-agent firecracker vm-assets
	@printf 'Agent ready: %s\n' "$(abspath $(AGENT_BIN))"

dirs:
	@mkdir -p "$(BIN_DIR)" "$(VM_DIR)"

build-agent: check-linux dirs
	@CGO_ENABLED=0 go build -trimpath -o "$(AGENT_BIN)" ./cmd

firecracker: check-linux check-tools dirs
	@latest="$(FIRECRACKER_VERSION)"; \
	if [ "$$latest" = "latest" ]; then \
		latest="$$(basename "$$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$(FIRECRACKER_RELEASES_URL)/latest")")"; \
	fi; \
	tmp_dir="$$(mktemp -d "$${TMPDIR:-/tmp}/dagflows-firecracker.XXXXXX")"; \
	trap 'rm -rf -- "$$tmp_dir"' EXIT; \
	archive="$$tmp_dir/firecracker-$${latest}-$(FIRECRACKER_ARCH).tgz"; \
	release_dir="$$tmp_dir/release-$${latest}-$(FIRECRACKER_ARCH)"; \
	printf 'Downloading Firecracker %s for %s\n' "$$latest" "$(FIRECRACKER_ARCH)"; \
	curl -fL "$(FIRECRACKER_RELEASES_URL)/download/$${latest}/firecracker-$${latest}-$(FIRECRACKER_ARCH).tgz" -o "$$archive"; \
	tar -xzf "$$archive" -C "$$tmp_dir"; \
	install -m 0755 "$$release_dir/firecracker-$${latest}-$(FIRECRACKER_ARCH)" "$(FIRECRACKER_BIN)"; \
	"$(FIRECRACKER_BIN)" --version

vm-assets: check-linux check-vm-tools dirs
	@project_dir="$(abspath .)" && \
	cd "$(VM_DIR)" && \
	ARCH="$(FIRECRACKER_ARCH)" && \
	S3="$(FIRECRACKER_CI_S3)" && \
	stage="$$(mktemp -d "$${TMPDIR:-/tmp}/dagflows-vm-assets.XXXXXX")" && \
	trap 'sudo rm -rf -- "$$stage"' EXIT && \
	case "$$ARCH" in x86_64) goarch=amd64 ;; aarch64) goarch=arm64 ;; *) printf 'Unsupported architecture: %s\n' "$$ARCH" >&2; exit 1 ;; esac && \
	printf 'Resolving latest Firecracker CI assets for %s\n' "$$ARCH" && \
	CI_ARTIFACTS_PREFIX="$$(curl -fsSL "$$S3?list-type=2&prefix=firecracker-ci/&delimiter=/" | grep -oP '(?<=<Prefix>)firecracker-ci/[0-9]{8}-[^/]+/(?=</Prefix>)' | sort | tail -1)" && \
	latest_kernel_key="$$(curl -fsSL "$$S3?list-type=2&prefix=$${CI_ARTIFACTS_PREFIX}$${ARCH}/vmlinux-" | grep -oP "(?<=<Key>)($${CI_ARTIFACTS_PREFIX}$${ARCH}/vmlinux-[0-9]+\.[0-9]+\.[0-9]{1,3})(?=</Key>)" | sort -V | tail -1)" && \
	latest_ubuntu_key="$$(curl -fsSL "$$S3?list-type=2&prefix=$${CI_ARTIFACTS_PREFIX}$${ARCH}/ubuntu-" | grep -oP "(?<=<Key>)($${CI_ARTIFACTS_PREFIX}$${ARCH}/ubuntu-[0-9]+\.[0-9]+\.squashfs)(?=</Key>)" | sort -V | tail -1)" && \
	ubuntu_version="$$(basename "$$latest_ubuntu_key" .squashfs | grep -oE '[0-9]+\.[0-9]+')" && \
	kernel_file="$$(basename "$$latest_kernel_key")" && \
	rootfs_file="ubuntu-$$ubuntu_version.ext4" && \
	curl -fL "$$S3/$$latest_kernel_key" -o "$$stage/$$kernel_file" && \
	curl -fL "$$S3/$$latest_ubuntu_key" -o "$$stage/rootfs.squashfs" && \
	unsquashfs -f -d "$$stage/rootfs" "$$stage/rootfs.squashfs" >/dev/null && \
	mkdir -p "$$stage/rootfs/usr/local/bin" "$$stage/rootfs/etc/systemd/system/multi-user.target.wants" && \
	CGO_ENABLED=0 GOOS=linux GOARCH="$$goarch" go build -C "$$project_dir" -trimpath -o "$$stage/rootfs/usr/local/bin/agent" ./internal/agent && \
	install -m 0644 "$$project_dir/internal/agent/agent.service" "$$stage/rootfs/etc/systemd/system/agent.service" && \
	ln -sfn ../agent.service "$$stage/rootfs/etc/systemd/system/multi-user.target.wants/agent.service" && \
	sudo chown -R root:root "$$stage/rootfs" && \
	truncate -s "$(ROOTFS_SIZE)" "$$stage/$$rootfs_file" && \
	sudo mkfs.ext4 -d "$$stage/rootfs" -F "$$stage/$$rootfs_file" >/dev/null && \
	sudo chown "$$(id -u):$$(id -g)" "$$stage/$$rootfs_file" && \
	e2fsck -fn "$$stage/$$rootfs_file" >/dev/null && \
	install -m 0644 "$$stage/$$kernel_file" "$$kernel_file" && \
	cp --sparse=always "$$stage/$$rootfs_file" "$$rootfs_file" && \
	chmod 0644 "$$rootfs_file" && \
	ln -sfn "$$kernel_file" vmlinux && \
	ln -sfn "$$rootfs_file" rootfs.ext4 && \
	printf 'Kernel: %s\nRootfs: %s\n' "$$(readlink vmlinux)" "$$(readlink rootfs.ext4)"

check-linux:
	@[ "$$(uname -s)" = "Linux" ] || { printf 'Firecracker setup requires Linux with KVM; got %s\n' "$$(uname -s)" >&2; exit 1; }

check-tools:
	@missing=0; \
	for cmd in curl tar install uname mkdir mktemp cp rm go; do \
		if ! command -v "$$cmd" >/dev/null 2>&1; then \
			printf 'Missing required command: %s\n' "$$cmd" >&2; \
			missing=1; \
		fi; \
	done; \
	exit "$$missing"

check-vm-tools: check-tools
	@missing=0; \
	for cmd in grep sort tail basename unsquashfs sudo truncate mkfs.ext4 e2fsck readlink ln chmod; do \
		if ! command -v "$$cmd" >/dev/null 2>&1; then \
			printf 'Missing required command for vm-assets: %s\n' "$$cmd" >&2; \
			missing=1; \
		fi; \
	done; \
	exit "$$missing"

kvm-check: check-linux
	@[ -e /dev/kvm ] || { printf '/dev/kvm is missing; enable KVM before running Firecracker\n' >&2; exit 1; }
	@if ! { [ -r /dev/kvm ] && [ -w /dev/kvm ]; }; then \
		user="$$(id -un)"; group="$$(stat -c '%G' /dev/kvm)"; \
		printf '/dev/kvm is not readable/writable by %s (device group: %s)\n' "$$user" "$$group" >&2; \
		printf 'Current session groups: %s\n' "$$(id -nG)" >&2; \
		if [ "$$group" = kvm ]; then \
			printf 'Group option: sudo usermod -aG kvm %s (then log out and back in)\n' "$$user" >&2; \
		fi; \
		printf 'ACL option: sudo setfacl -m u:%s:rw /dev/kvm\n' "$$user" >&2; \
		exit 1; \
	fi
	@printf '/dev/kvm is accessible\n'

SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c

LOCAL_DIR ?= .local
BIN_DIR ?= $(LOCAL_DIR)/bin
VM_DIR ?= $(LOCAL_DIR)/vm
TMP_DIR ?= $(LOCAL_DIR)/tmp

FIRECRACKER_RELEASES_URL ?= https://github.com/firecracker-microvm/firecracker/releases
FIRECRACKER_VERSION ?= latest
FIRECRACKER_ARCH ?= x86_64
FIRECRACKER_BIN := $(BIN_DIR)/firecracker
JAILER_BIN := $(BIN_DIR)/jailer

FIRECRACKER_CI_S3 ?= https://s3.amazonaws.com/spec.ccfc.min
ROOTFS_SIZE ?= 1G

.PHONY: help setup dirs env firecracker vm-assets check-linux check-tools check-vm-tools kvm-check print-env

help:
	@printf '%s\n' \
		'Targets:' \
		'  make setup       Check KVM, create env, download Firecracker and VM assets' \
		'  make firecracker Download Firecracker and jailer into .local/bin' \
		'  make vm-assets   Download kernel/rootfs into .local/vm (requires sudo)' \
		'  make kvm-check   Verify /dev/kvm exists and is accessible' \
		'' 

setup: check-linux kvm-check check-tools check-vm-tools dirs env firecracker vm-assets
	@printf 'Worker environment ready. Firecracker: %s\n' "$(abspath $(FIRECRACKER_BIN))"

dirs:
	@mkdir -p "$(BIN_DIR)" "$(VM_DIR)" "$(TMP_DIR)"

env:
	@if [ ! -f .env ]; then \
		cp .env.example .env; \
		printf 'Created .env from .env.example\n'; \
	else \
		printf '.env already exists; leaving it unchanged\n'; \
	fi

firecracker: check-linux check-tools dirs
	@latest="$(FIRECRACKER_VERSION)"; \
	if [ "$$latest" = "latest" ]; then \
		latest="$$(basename "$$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$(FIRECRACKER_RELEASES_URL)/latest")")"; \
	fi; \
	archive="$(TMP_DIR)/firecracker-$${latest}-$(FIRECRACKER_ARCH).tgz"; \
	release_dir="$(TMP_DIR)/release-$${latest}-$(FIRECRACKER_ARCH)"; \
	printf 'Downloading Firecracker %s for %s\n' "$$latest" "$(FIRECRACKER_ARCH)"; \
	curl -fL "$(FIRECRACKER_RELEASES_URL)/download/$${latest}/firecracker-$${latest}-$(FIRECRACKER_ARCH).tgz" -o "$$archive"; \
	rm -rf "$$release_dir"; \
	tar -xzf "$$archive" -C "$(TMP_DIR)"; \
	install -m 0755 "$$release_dir/firecracker-$${latest}-$(FIRECRACKER_ARCH)" "$(FIRECRACKER_BIN)"; \
	if [ -f "$$release_dir/jailer-$${latest}-$(FIRECRACKER_ARCH)" ]; then \
		install -m 0755 "$$release_dir/jailer-$${latest}-$(FIRECRACKER_ARCH)" "$(JAILER_BIN)"; \
	fi; \
	"$(FIRECRACKER_BIN)" --version

vm-assets: check-linux check-vm-tools dirs
	@cd "$(VM_DIR)" && \
	ARCH="$(FIRECRACKER_ARCH)" && \
	S3="$(FIRECRACKER_CI_S3)" && \
	printf 'Resolving latest Firecracker CI assets for %s\n' "$$ARCH" && \
	CI_ARTIFACTS_PREFIX="$$(curl -fsSL "$$S3?list-type=2&prefix=firecracker-ci/&delimiter=/" | grep -oP '(?<=<Prefix>)firecracker-ci/[0-9]{8}-[^/]+/(?=</Prefix>)' | sort | tail -1)" && \
	latest_kernel_key="$$(curl -fsSL "$$S3?list-type=2&prefix=$${CI_ARTIFACTS_PREFIX}$${ARCH}/vmlinux-" | grep -oP "(?<=<Key>)($${CI_ARTIFACTS_PREFIX}$${ARCH}/vmlinux-[0-9]+\.[0-9]+\.[0-9]{1,3})(?=</Key>)" | sort -V | tail -1)" && \
	latest_ubuntu_key="$$(curl -fsSL "$$S3?list-type=2&prefix=$${CI_ARTIFACTS_PREFIX}$${ARCH}/ubuntu-" | grep -oP "(?<=<Key>)($${CI_ARTIFACTS_PREFIX}$${ARCH}/ubuntu-[0-9]+\.[0-9]+\.squashfs)(?=</Key>)" | sort -V | tail -1)" && \
	ubuntu_version="$$(basename "$$latest_ubuntu_key" .squashfs | grep -oE '[0-9]+\.[0-9]+')" && \
	kernel_file="$$(basename "$$latest_kernel_key")" && \
	rootfs_file="ubuntu-$$ubuntu_version.ext4" && \
	curl -fL "$$S3/$$latest_kernel_key" -o "$$kernel_file" && \
	curl -fL "$$S3/$$latest_ubuntu_key" -o "ubuntu-$$ubuntu_version.squashfs.upstream" && \
	rm -rf squashfs-root && \
	unsquashfs -f "ubuntu-$$ubuntu_version.squashfs.upstream" >/dev/null && \
	if [ ! -f id_rsa ]; then ssh-keygen -f id_rsa -N '' >/dev/null; fi && \
	mkdir -p squashfs-root/root/.ssh && \
	cp id_rsa.pub squashfs-root/root/.ssh/authorized_keys && \
	sudo chown -R root:root squashfs-root && \
	truncate -s "$(ROOTFS_SIZE)" "$$rootfs_file" && \
	sudo mkfs.ext4 -d squashfs-root -F "$$rootfs_file" >/dev/null && \
	ln -sfn "$$kernel_file" vmlinux && \
	ln -sfn "$$rootfs_file" rootfs.ext4 && \
	e2fsck -fn "$$rootfs_file" >/dev/null && \
	printf 'Kernel: %s\nRootfs: %s\nSSH key: %s\n' "$$(readlink vmlinux)" "$$(readlink rootfs.ext4)" "$$(pwd)/id_rsa"

check-linux:
	@[ "$$(uname -s)" = "Linux" ] || { printf 'Firecracker setup requires Linux with KVM; got %s\n' "$$(uname -s)" >&2; exit 1; }

check-tools:
	@missing=0; \
	for cmd in curl tar install uname mkdir cp rm; do \
		if ! command -v "$$cmd" >/dev/null 2>&1; then \
			printf 'Missing required command: %s\n' "$$cmd" >&2; \
			missing=1; \
		fi; \
	done; \
	exit "$$missing"

check-vm-tools: check-tools
	@missing=0; \
	for cmd in grep sort tail basename unsquashfs ssh-keygen sudo truncate e2fsck; do \
		if ! command -v "$$cmd" >/dev/null 2>&1; then \
			printf 'Missing required command for vm-assets: %s\n' "$$cmd" >&2; \
			missing=1; \
		fi; \
	done; \
	exit "$$missing"

kvm-check: check-linux
	@[ -e /dev/kvm ] || { printf '/dev/kvm is missing; enable KVM before running Firecracker\n' >&2; exit 1; }
	@[ -r /dev/kvm ] && [ -w /dev/kvm ] || { printf '/dev/kvm exists but is not readable/writable by this user\n' >&2; exit 1; }
	@printf '/dev/kvm is accessible\n'

print-env:
	@printf 'FIRECRACKER_BIN=%s\n' "$(abspath $(FIRECRACKER_BIN))"
	@printf 'JAILER_BIN=%s\n' "$(abspath $(JAILER_BIN))"
	@printf 'VM_DIR=%s\n' "$(abspath $(VM_DIR))"

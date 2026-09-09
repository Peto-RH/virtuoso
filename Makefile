OUTPUT_DIR=bin
RPMBUILD_DIR=$(CURDIR)/rpmbuild
VERSION := $(shell rpmspec virtuoso.spec --query --srpm --queryformat '%{version}')

BUILDROOT ?=
BINDIR ?= /usr/bin
UNITDIR ?= /usr/lib/systemd/system
SYSUSERSDIR ?= /usr/lib/sysusers.d
SYSCONFDIR ?= /etc

.PHONY: build
build:
	mkdir -p $(OUTPUT_DIR)
	go build -o $(OUTPUT_DIR)/virtuoso ./cmd/virtuoso

.PHONY: clean
clean:
	rm -rf $(OUTPUT_DIR) $(RPMBUILD_DIR)

.PHONY: install_files
install_files:
	install -D -m 0755 bin/virtuoso "$(BUILDROOT)$(BINDIR)/virtuoso"
	install -D -m 0644 data/systemd/virtuoso.service "$(BUILDROOT)$(UNITDIR)/virtuoso.service"
	install -D -m 0644 data/systemd/virtuoso.timer "$(BUILDROOT)$(UNITDIR)/virtuoso.timer"
	install -D -m 0644 data/sysusers.d/virtuoso.conf "$(BUILDROOT)$(SYSUSERSDIR)/virtuoso.conf"
	install -d -m 0750 "$(BUILDROOT)$(SYSCONFDIR)/virtuoso"
	install -D -m 0640 config/virtuoso.toml "$(BUILDROOT)$(SYSCONFDIR)/virtuoso/virtuoso.toml"

.PHONY: source-tarball
source-tarball:
	mkdir -p "$(RPMBUILD_DIR)/SOURCES"
	git -c safe.directory="$(CURDIR)" archive \
		--format=tar.gz \
		--prefix="virtuoso-$(VERSION)/" \
		--output="$(RPMBUILD_DIR)/SOURCES/virtuoso-$(VERSION).tar.gz" \
		HEAD

.PHONY: rpm
rpm: source-tarball
	rpmbuild -ba virtuoso.spec --define "_topdir $(RPMBUILD_DIR)"

.PHONY: test
test:
	go test ./...

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: lint-fix
lint-fix:
	golangci-lint run --fix ./...

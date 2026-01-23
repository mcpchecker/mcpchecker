AGENT_BINARY_NAME = agent
MCPCHECKER_BINARY_NAME = mcpchecker
MOCK_AGENT_BINARY_NAME = functional/mock-agent

# Release build variables (can be overridden)
VERSION ?= dev
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

# Changelog parsing pipeline: removes section boundaries, formats sections with their items
define CHANGELOG_PIPELINE
sed '$$d' | tail -n +2 | sed -e '$$G' | awk '/^### /{section=$$0; items=""; next} /^( *)?- /{items=items $$0 "\n"; next} /^$$/ && items{print section "\n" items; items=""}' | sed '/^$$/d'
endef

.PHONY: clean
clean:
	rm -f $(AGENT_BINARY_NAME) $(MCPCHECKER_BINARY_NAME) $(MOCK_AGENT_BINARY_NAME)
	rm -f *.zip *.bundle

.PHONY: build-agent
build-agent: clean
	go build -o $(AGENT_BINARY_NAME) ./cmd/agent

.PHONY: build-mcpchecker
build-mcpchecker: clean
	go build -o $(MCPCHECKER_BINARY_NAME) ./cmd/mcpchecker/

.PHONY: build
build: build-agent build-mcpchecker

.PHONY: test
test:
	go test ./...

# Internal target - builds mock agent for functional tests
.PHONY: _build-mock-agent
_build-mock-agent:
	go build -o $(MOCK_AGENT_BINARY_NAME) ./functional/servers/agent/cmd

.PHONY: functional
functional: build _build-mock-agent ## Run functional tests
	MCPCHECKER_BINARY=$(CURDIR)/mcpchecker MOCK_AGENT_BINARY=$(CURDIR)/$(MOCK_AGENT_BINARY_NAME) go test -v -tags functional ./functional/...

.PHONY: functional-coverage
functional-coverage: clean _build-mock-agent ## Run functional tests with coverage (in-process mode)
	MCPCHECKER_TEST_INPROCESS=true \
	MOCK_AGENT_BINARY=$(CURDIR)/$(MOCK_AGENT_BINARY_NAME) \
	go test -v -tags functional -cover -coverprofile=$(CURDIR)/coverage.out -coverpkg=./pkg/...,./cmd/... ./functional/... -p=1
	go tool cover -html=$(CURDIR)/coverage.out -o=$(CURDIR)/coverage.html
	@echo "Coverage report: $(CURDIR)/coverage.html"

# Release targets for CI/CD
.PHONY: build-release
build-release:
	@echo "Building release binaries for $(GOOS)/$(GOARCH)..."
	@mkdir -p dist
	@if [ "$(GOOS)" = "windows" ]; then \
		GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -ldflags="-s -w" -o "dist/$(MCPCHECKER_BINARY_NAME)-$(GOOS)-$(GOARCH).exe" ./cmd/mcpchecker; \
		GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -ldflags="-s -w" -o "dist/$(AGENT_BINARY_NAME)-$(GOOS)-$(GOARCH).exe" ./cmd/agent; \
	else \
		GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -ldflags="-s -w" -o "dist/$(MCPCHECKER_BINARY_NAME)-$(GOOS)-$(GOARCH)" ./cmd/mcpchecker; \
		GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -ldflags="-s -w" -o "dist/$(AGENT_BINARY_NAME)-$(GOOS)-$(GOARCH)" ./cmd/agent; \
	fi
	@echo "Build complete!"

.PHONY: package-release
package-release:
	@echo "Packaging release artifacts for $(GOOS)/$(GOARCH)..."
	@cd dist && \
	if [ "$(GOOS)" = "windows" ]; then \
		zip "$(MCPCHECKER_BINARY_NAME)-$(GOOS)-$(GOARCH).zip" "$(MCPCHECKER_BINARY_NAME)-$(GOOS)-$(GOARCH).exe"; \
		zip "$(AGENT_BINARY_NAME)-$(GOOS)-$(GOARCH).zip" "$(AGENT_BINARY_NAME)-$(GOOS)-$(GOARCH).exe"; \
	else \
		zip "$(MCPCHECKER_BINARY_NAME)-$(GOOS)-$(GOARCH).zip" "$(MCPCHECKER_BINARY_NAME)-$(GOOS)-$(GOARCH)"; \
		zip "$(AGENT_BINARY_NAME)-$(GOOS)-$(GOARCH).zip" "$(AGENT_BINARY_NAME)-$(GOOS)-$(GOARCH)"; \
	fi
	@echo "Packaging complete!"

.PHONY: sign-release
sign-release:
	@echo "Signing release artifacts for $(GOOS)/$(GOARCH)..."
	@cd dist && \
	cosign sign-blob --yes "$(MCPCHECKER_BINARY_NAME)-$(GOOS)-$(GOARCH).zip" \
		--bundle "$(MCPCHECKER_BINARY_NAME)-$(GOOS)-$(GOARCH).zip.bundle" && \
	cosign sign-blob --yes "$(AGENT_BINARY_NAME)-$(GOOS)-$(GOARCH).zip" \
		--bundle "$(AGENT_BINARY_NAME)-$(GOOS)-$(GOARCH).zip.bundle"
	@echo "Signing complete!"

.PHONY: release
release: build-release package-release sign-release
	@echo "Release build complete for $(GOOS)/$(GOARCH)!"

# Changelog extraction targets
.PHONY: extract-changelog-unreleased
extract-changelog-unreleased:
	@echo "Extracting unreleased changelog section..." >&2
	@CHANGELOG_CONTENT=$$(sed -n '/## \[Unreleased\]/,/## \[/p' CHANGELOG.md | $(CHANGELOG_PIPELINE)); \
	if [ -z "$$CHANGELOG_CONTENT" ]; then \
		CHANGELOG_CONTENT="See CHANGELOG.md for details."; \
	fi; \
	printf '%s\n' "$$CHANGELOG_CONTENT"

.PHONY: extract-changelog-version
extract-changelog-version:
	@if [ -z "$(VERSION)" ]; then \
		echo "Error: VERSION is required. Usage: make extract-changelog-version VERSION=v1.0.0"; \
		exit 1; \
	fi
	@echo "Extracting changelog for version $(VERSION)..." >&2
	@VERSION_NO_V=$$(echo "$(VERSION)" | sed 's/^v//'); \
	CHANGELOG_CONTENT=$$(sed -n "/## \[$${VERSION_NO_V}\]/,/## \[/p" CHANGELOG.md | $(CHANGELOG_PIPELINE)); \
	if [ -z "$$CHANGELOG_CONTENT" ]; then \
		CHANGELOG_CONTENT=$$(sed -n '/## \[Unreleased\]/,/## \[/p' CHANGELOG.md | $(CHANGELOG_PIPELINE)); \
	fi; \
	if [ -z "$$CHANGELOG_CONTENT" ]; then \
		CHANGELOG_CONTENT="See CHANGELOG.md for details."; \
	fi; \
	printf '%s\n' "$$CHANGELOG_CONTENT"

# Version validation targets
.PHONY: validate-version-tag
validate-version-tag:
	@if [ -z "$(VERSION)" ]; then \
		echo "Error: VERSION is required. Usage: make validate-version-tag VERSION=v1.0.0"; \
		exit 1; \
	fi
	@echo "Validating version tag format: $(VERSION)"
	@if echo "$(VERSION)" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+(-rc\.[0-9]+)?$$'; then \
		echo "✓ Version tag $(VERSION) is valid"; \
	else \
		echo "✗ Error: Version tag must match format 'vX.Y.Z' or 'vX.Y.Z-rc.N'"; \
		echo "  Got: $(VERSION)"; \
		exit 1; \
	fi

.PHONY: validate-release-tag
validate-release-tag:
	@if [ -z "$(VERSION)" ]; then \
		echo "Error: VERSION is required. Usage: make validate-release-tag VERSION=v1.0.0"; \
		exit 1; \
	fi
	@echo "Validating release tag format: $(VERSION)"
	@if echo "$(VERSION)" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+$$'; then \
		echo "✓ Release tag $(VERSION) is valid"; \
	else \
		echo "✗ Error: Release tag must match format 'vX.Y.Z' (no suffixes)"; \
		echo "  Got: $(VERSION)"; \
		exit 1; \
	fi

.PHONY: validate-prerelease-tag
validate-prerelease-tag:
	@if [ -z "$(VERSION)" ]; then \
		echo "Error: VERSION is required. Usage: make validate-prerelease-tag VERSION=v1.0.0-rc.1"; \
		exit 1; \
	fi
	@echo "Validating prerelease tag format: $(VERSION)"
	@if echo "$(VERSION)" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+-rc\.[0-9]+$$'; then \
		echo "✓ Prerelease tag $(VERSION) is valid"; \
	else \
		echo "✗ Error: Prerelease tag must match format 'vX.Y.Z-rc.N'"; \
		echo "  Got: $(VERSION)"; \
		exit 1; \
	fi

.PHONY: validate-changelog-has-version
validate-changelog-has-version:
	@if [ -z "$(VERSION)" ]; then \
		echo "Error: VERSION is required. Usage: make validate-changelog-has-version VERSION=v1.0.0"; \
		exit 1; \
	fi
	@VERSION_NO_V=$$(echo "$(VERSION)" | sed 's/^v//'); \
	echo "Checking if CHANGELOG.md contains section for version $${VERSION_NO_V}..."; \
	if grep -q "## \[$${VERSION_NO_V}\]" CHANGELOG.md; then \
		echo "✓ CHANGELOG.md contains section for version $${VERSION_NO_V}"; \
	else \
		echo "✗ Error: CHANGELOG.md must contain a section for version $${VERSION_NO_V}"; \
		echo "  Expected format: ## [$${VERSION_NO_V}]"; \
		echo "  Current CHANGELOG sections:"; \
		grep "^## \[" CHANGELOG.md || echo "  No version sections found"; \
		exit 1; \
	fi

# Release management targets
.PHONY: upload-release-assets
upload-release-assets:
	@if [ -z "$(VERSION)" ]; then \
		echo "Error: VERSION is required. Usage: make upload-release-assets VERSION=v1.0.0"; \
		exit 1; \
	fi
	@if [ -z "$(GITHUB_TOKEN)" ]; then \
		echo "Error: GITHUB_TOKEN environment variable is required"; \
		exit 1; \
	fi
	@echo "Uploading release assets for $(VERSION)..."
	@for file in dist/*.zip dist/*.bundle; do \
		if [ -f "$$file" ]; then \
			echo "Uploading $$file..."; \
			gh release upload "$(VERSION)" "$$file" --clobber; \
		fi; \
	done
	@echo "✓ All assets uploaded successfully"


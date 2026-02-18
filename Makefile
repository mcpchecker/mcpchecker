AGENT_BINARY_NAME = agent
MCPCHECKER_BINARY_NAME = mcpchecker
MOCK_AGENT_BINARY_NAME = functional/mock-agent

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "development")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "")
LDFLAGS = -X github.com/mcpchecker/mcpchecker/pkg/cli.Version=$(VERSION) -X github.com/mcpchecker/mcpchecker/pkg/cli.Commit=$(COMMIT)

.PHONY: clean
clean:
	rm -f $(AGENT_BINARY_NAME) $(MCPCHECKER_BINARY_NAME) $(MOCK_AGENT_BINARY_NAME)
	rm -f *.zip *.bundle

.PHONY: build-agent
build-agent: clean
	go build -o $(AGENT_BINARY_NAME) ./cmd/agent

.PHONY: build-mcpchecker
build-mcpchecker: clean
	go build -ldflags "$(LDFLAGS)" -o $(MCPCHECKER_BINARY_NAME) ./cmd/mcpchecker/

.PHONY: build
build: build-agent build-mcpchecker

.PHONY: test
test:
	go test -count=1 -race ./...

# Internal target - builds mock agent for functional tests
.PHONY: _build-mock-agent
_build-mock-agent:
	go build -o $(MOCK_AGENT_BINARY_NAME) ./functional/servers/agent/cmd

.PHONY: functional
functional: build _build-mock-agent ## Run functional tests
	MCPCHECKER_BINARY=$(CURDIR)/mcpchecker MOCK_AGENT_BINARY=$(CURDIR)/$(MOCK_AGENT_BINARY_NAME) go test -v -count=1 -race -tags functional ./functional/...

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
AGENT_TARGETS := darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 linux/arm
SERVER_TARGETS := linux/amd64 linux/arm64
WEBUI := server/internal/webui/dist

.PHONY: all dev test build server agent agents web release clean

all: test build

# Run controller, agent and web UI together.
dev:
	./scripts/dev.sh

test:
	cd agent && go vet ./... && go test -race ./...
	cd server && go vet ./... && go test -race ./...
	cd web && npm run build

build: web server agent

server:
	cd server && CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o ../bin/cockpit-server ./cmd/cockpit-server

agent:
	cd agent && CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o ../bin/cockpit-agent ./cmd/cockpit-agent

# Build the UI and copy it into the server package for embedding.
web:
	cd web && npm ci && npm run build
	find $(WEBUI) -mindepth 1 ! -name .gitkeep -delete
	cp -R web/dist/. $(WEBUI)/

# Cross-compile the agent for every supported host.
agents:
	@for t in $(AGENT_TARGETS); do \
		os=$${t%/*}; arch=$${t#*/}; \
		echo "building cockpit-agent-$$os-$$arch"; \
		(cd agent && CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch GOARM=7 go build -ldflags "$(LDFLAGS)" -o ../bin/cockpit-agent-$$os-$$arch ./cmd/cockpit-agent) || exit 1; \
	done

# Everything needed to deploy: controller with embedded UI, all agents, and
# checksums.txt, which install.sh verifies downloads against.
release: web agents
	@for t in $(SERVER_TARGETS); do \
		os=$${t%/*}; arch=$${t#*/}; \
		echo "building cockpit-server-$$os-$$arch"; \
		(cd server && CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -ldflags "$(LDFLAGS)" -o ../bin/cockpit-server-$$os-$$arch ./cmd/cockpit-server) || exit 1; \
	done
	cd bin && shasum -a 256 cockpit-agent-* cockpit-server-*-* > checksums.txt

clean:
	rm -rf bin web/dist
	find $(WEBUI) -mindepth 1 ! -name .gitkeep -delete

SHELL := /bin/sh
VERSION ?= 0.1.0

.PHONY: all build test test-deployment test-integration fmt vet clean dev-controller dev-agent dev-launcher seed-dev reset-dev controller agent launcher

all: test build

build: controller agent launcher

controller:
	mkdir -p bin
	cd controller && go build -ldflags "-X main.version=$(VERSION)" -o ../bin/thinpi-controller ./cmd/thinpi-controller

agent:
	mkdir -p bin
	cd agent && go build -ldflags "-X main.version=$(VERSION)" -o ../bin/thinpi-agent ./cmd/thinpi-agent

launcher:
	cmake -S launcher -B build/launcher -G Ninja -DCMAKE_BUILD_TYPE=Release
	cmake --build build/launcher

test:
	cd controller && go test ./...
	cd agent && go test ./...
	sh ./tests/deployment/client-scripts.sh
	@if command -v cmake >/dev/null 2>&1 && pkg-config --exists Qt6Core Qt6Test 2>/dev/null; then cmake -S launcher -B build/launcher-tests -G Ninja -DBUILD_TESTING=ON && cmake --build build/launcher-tests && ctest --test-dir build/launcher-tests --output-on-failure; else echo "Qt 6 not installed; launcher tests skipped"; fi

test-deployment:
	sh ./tests/deployment/client-scripts.sh

test-integration: agent
	./tests/integration/mock-flow.sh

fmt:
	cd controller && gofmt -w .
	cd agent && gofmt -w .

vet:
	cd controller && go vet ./...
	cd agent && go vet ./...

dev-controller:
	cd controller && go run ./cmd/thinpi-controller serve --dev --listen 127.0.0.1:8080 --database ../thinpi-dev.db

dev-agent:
	cd agent && THINPI_AGENT_MOCK_CLIENTS=1 go run ./cmd/thinpi-agent serve --config ../deploy/pi/agent-dev.json

dev-launcher:
	THINPI_DEV_MODE=1 THINPI_API_URL=http://127.0.0.1:8080 THINPI_AGENT_SOCKET=/tmp/thinpi-agent-dev.sock ./build/launcher/thinpi-launcher

seed-dev:
	cd controller && go run ./cmd/thinpi-controller seed-dev --database ../thinpi-dev.db --dev

reset-dev:
	cd controller && go run ./cmd/thinpi-controller reset-dev --database ../thinpi-dev.db --dev

clean:
	rm -rf bin build

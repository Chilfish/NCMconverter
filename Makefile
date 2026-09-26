# The command name is also the package directory under cmd/.
CMD := ncmconverter
# Windows needs the extension on the built file; everywhere else the bare name
# stands alone.
BINARY := $(CMD)
ifeq ($(OS),Windows_NT)
BINARY := $(CMD).exe
endif

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || echo none)
DATE ?= $(shell git log -1 --format=%cI 2>/dev/null || echo unknown)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: all build install run test test-race coverage vet fmt fmt-check lint clean

all: build

# build: compile the binary into bin/
build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/$(CMD)

# install: install the binary into GOBIN
install:
	go install -trimpath -ldflags "$(LDFLAGS)" ./cmd/$(CMD)

# run: build and then run; pass arguments with ARGS="..."
run: build
	./bin/$(BINARY) $(ARGS)

# test: run the test suite
test:
	go test ./...

# test-race: run the test suite under the race detector
test-race:
	go test -race ./...

# coverage: write a coverage profile to coverage.out
coverage:
	go test -covermode=atomic -coverprofile=coverage.out ./...

# vet: run go vet
vet:
	go vet ./...

# fmt: format the tree
fmt:
	gofmt -w .

# fmt-check: fail when anything is unformatted
fmt-check:
	@test -z "$$(gofmt -l .)" || { gofmt -l .; echo "run make fmt"; exit 1; }

# lint: run golangci-lint
lint:
	golangci-lint run

# clean: remove build output only, never the audio or the containers
clean:
	-rm -rf bin
	-rm -f coverage.out

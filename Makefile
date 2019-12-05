SHELL:=/bin/bash

PROJECT_ROOT:=src/github.com/CSCfi/qvain-api
GOPATH:=$(shell pwd)
PATH:=$(GOPATH)/bin:$(PATH)

### VCS
TAG := $(shell git describe --tags --always --dirty="-dev" 2>/dev/null)
HASH := $(shell git rev-parse --short HEAD 2>/dev/null)
BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null)
REPO := $(shell git ls-remote --get-url 2>/dev/null)
REPOLINK := $(shell test -x $(SOURCELINK) && bin/sourcelink $(REPO) $(HASH) $(BRANCH) 2>/dev/null || echo)
VERSION_PACKAGE := $(shell go list -f '{{.ImportPath}}' ./src/github.com/CSCfi/qvain-api/internal/version)

### collect VCS info for linker
LDFLAGS := "-s -w -X $(VERSION_PACKAGE).CommitHash=$(HASH) -X $(VERSION_PACKAGE).CommitTag=$(TAG) -X $(VERSION_PACKAGE).CommitBranch=$(BRANCH) -X $(VERSION_PACKAGE).CommitRepo=$(REPOLINK)"

GO_MAKE_BUILD:=go get -d && go build -v -ldflags $(LDFLAGS) && go install

export PATH
export GOPATH

all: sourcelink summary es-cli message metax-cli qvain-backend qvain-cli redis-test
	@echo
	@echo "Build complete."
	@echo

summary:
	@echo =========================================================
	@echo TAG: $(TAG)
	@echo HASH: $(HASH)
	@echo BRANCH: $(BRANCH)
	@echo REPO: $(REPO)
	@echo REPOLINK: $(REPOLINK)
	@echo VERSION_PACKAGE: $(VERSION_PACKAGE)
	@echo =========================================================

es-cli:
	@echo "Building es-cli.."
	@cd $(PROJECT_ROOT)/cmd/es-cli && $(GO_MAKE_BUILD)
	@echo "..built."

message:
	@echo "Building message.."
	@cd $(PROJECT_ROOT)/cmd/message && $(GO_MAKE_BUILD)
	@echo "..built."

metax-cli:
	@echo "Building metax-cli.."
	@cd $(PROJECT_ROOT)/cmd/metax-cli && $(GO_MAKE_BUILD)
	@echo "..built."

qvain-backend:
	@echo "Building qvain-backend.."
	@cd $(PROJECT_ROOT)/cmd/qvain-backend && $(GO_MAKE_BUILD)
	@echo "..built."

qvain-cli:
	@echo "Building qvain-cli.."
	@cd $(PROJECT_ROOT)/cmd/qvain-cli && $(GO_MAKE_BUILD)
	@echo "..built."

redis-test:
	@echo "Building redis-test.."
	@cd $(PROJECT_ROOT)/cmd/redis-test && $(GO_MAKE_BUILD)
	@echo "..built."

sourcelink:
	@echo "Building sourcelink.."
	@cd $(PROJECT_ROOT)/cmd/sourcelink && $(GO_MAKE_BUILD)
	@echo "..built."

clean:
	chmod -Rf 700 pkg
	rm -Rf bin pkg

check: lint staticcheck gosec
	@echo
	@echo "== Running tests =="
	-@cd $(PROJECT_ROOT) && go test ./...
	@echo "== Completed tests =="
	@echo

security: gosec

lint:
	@echo
	@echo "== Ensure golint is installed =="
	@test -f bin/golint || go get -u golang.org/x/lint/golint 2> /dev/null
	@echo "== Completed golint installation =="
	@echo
	@echo "== Running golint =="
	@./bin/golint src/github.com/CSCfi/qvain-api/...
	@echo "== Completed golint =="
	@echo

staticcheck:
	@echo
	@echo "== Installing staticcheck =="
	@test -f bin/staticcheck || go get -u honnef.co/go/tools/cmd/staticcheck 2> /dev/null
	@echo "== Completed staticcheck installation =="
	@echo
	@echo "== Running staticcheck =="
	-@./bin/staticcheck -f stylish src/github.com/CSCfi/qvain-api/...
	@echo "== Completed staticcheck =="
	@echo

gosec:
	@echo
	@echo "== Installing gosec =="
	@test -f bin/gosec || go get github.com/securego/gosec/cmd/gosec
	@echo "== Completed gosec installation =="
	@echo
	@echo "== Running gosec =="
	-@./bin/gosec src/github.com/CSCfi/qvain-api/...
	@echo "== Completed gosec =="

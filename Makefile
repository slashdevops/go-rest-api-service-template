.DELETE_ON_ERROR: clean

EXECUTABLES = go zip shasum podman
K := $(foreach exec,$(EXECUTABLES),\
  $(if $(shell which $(exec)),some string,$(error "No $(exec) in PATH)))

# Needed only by the dev-env targets, so they are checked there rather than on
# every `make`: CI runs lint and test without ever rendering the pod file.
# envsubst is not on a stock macOS -- it comes with gettext.
DEV_ENV_EXECUTABLES = openssl envsubst

# Add .SHELLFLAGS to ensure that shell errors are propagated
.SHELLFLAGS := -e -c

# Used by rename-project: the owner and name of the repository that was created
# from the template, read from the origin remote. Both URL shapes GitHub hands
# out are handled -- `git@github.com:owner/name.git` and
# `https://github.com/owner/name.git`. The old `cut -d '/' -f 2` handled only
# the first: on an https URL the second slash-separated field is EMPTY, so the
# rename would have replaced the template name with nothing at all.
#
# Override both when there is no remote (a zip download, a fresh `git init`):
#   GIT_REPOSITORY_OWNER=acme GIT_REPOSITORY_NAME=my-api make rename-project
GIT_REPOSITORY_URL             := $(shell git remote get-url origin 2>/dev/null)
# No parentheses in these sed expressions: make counts them inside $(shell)
# and a capture group reads as an unterminated call.
GIT_REPOSITORY_OWNER           ?= $(shell echo '$(GIT_REPOSITORY_URL)' | sed -E 's|\.git$$||; s|/[^/]*$$||; s|^.*[:/]||')
GIT_REPOSITORY_NAME            ?= $(shell echo '$(GIT_REPOSITORY_URL)' | sed -E 's|\.git$$||; s|^.*/||')
GIT_REPOSITORY_NAME_UNDERSCORE := $(subst -,_,$(GIT_REPOSITORY_NAME))

# PROJECT_MODULE is the full module path, major-version suffix included
# (github.com/slashdevops/go-rest-api-service-template/v2). It is read from go.mod rather than rebuilt from
# the namespace and name, because the -ldflags -X symbol paths below must match the module
# path exactly: a stale path names a symbol that does not exist, the linker ignores it
# silently, and the binaries ship reporting version 0.0.0.
PROJECT_MODULE    ?= $(shell awk '/^module /{print $$2}' go.mod)
# The name and namespace are the third and second segments of the module path
# (github.com/<namespace>/<name>[/vN]), split from PROJECT_MODULE rather than
# grepped out of go.mod. They used to be `grep module go.mod | cut ...`, which
# matches EVERY line containing the word "module" -- and the day a comment
# mentioning "the module graph" was added to go.mod, PROJECT_NAME became two
# lines of prose, the backticks in it ran as a command, and every dev-env
# target died with `/bin/sh: -u: command not found` and `Error 127`. The guard
# turns that class of mistake into a message naming the variable.
PROJECT_NAME      ?= $(word 3,$(subst /, ,$(PROJECT_MODULE)))
PROJECT_NAMESPACE ?= $(word 2,$(subst /, ,$(PROJECT_MODULE)))
ifneq ($(words $(PROJECT_NAME)),1)
$(error PROJECT_NAME resolved to "$(PROJECT_NAME)"; expected exactly one word from the module line of go.mod)
endif
PROJECT_MODULES_PATH := $(shell ls -d cmd/*)
PROJECT_MODULES_NAME := $(foreach dir_name, $(PROJECT_MODULES_PATH), $(shell basename $(dir_name)) )
PROJECT_DEPENDENCIES := $(shell go list -m -f '{{if not (or .Indirect .Main)}}{{.Path}}{{end}}' all)

# What rename-project replaces: the template's own owner/name. Kept as ONE
# string on purpose -- the rename rewrites this line along with every other
# occurrence, so after a rename it names the new repository and the target
# becomes a no-op the second time. It had drifted to a name that appeared
# nowhere in the tree, so the target renamed nothing and then failed moving a
# cmd/ directory that did not exist.
TEMPLATE_REPOSITORY      := slashdevops/go-rest-api-service-template
TEMPLATE_NAME            := $(notdir $(TEMPLATE_REPOSITORY))
TEMPLATE_NAME_UNDERSCORE := $(subst -,_,$(TEMPLATE_NAME))

BUILD_DIR       := ./build
DIST_DIR        := ./dist
DIST_ASSETS_DIR := $(DIST_DIR)/assets

# Third-party license reporting. The module itself is ignored: go-licenses would
# otherwise try to classify our own packages as vendored dependencies.
LICENSES_DIR    ?= $(BUILD_DIR)/licenses
LICENSES_IGNORE := $(PROJECT_MODULE)

PROJECT_COVERAGE_FILE ?= $(BUILD_DIR)/coverage.txt
PROJECT_COVERAGE_MODE	?= atomic
PROJECT_COVERAGE_TAGS ?= unit
PROJECT_INTEGRATION_COVERAGE_TAGS ?= integration

GIT_VERSION     ?= $(shell git rev-parse --abbrev-ref HEAD | cut -d "/" -f 2)
GIT_COMMIT      ?= $(shell git rev-parse HEAD | tr -d '\040\011\012\015\n')
GIT_BRANCH      ?= $(shell git rev-parse --abbrev-ref HEAD | tr -d '\040\011\012\015\n')
GIT_USER        := $(shell git config --get user.name | tr -d '\040\011\012\015\n')
GIT_USER_EMAIL  := $(shell git config --get user.email | tr -d '\040\011\012\015\n')
BUILD_DATE      := $(shell date +'%Y-%m-%dT%H:%M:%S')

GO_LDFLAGS_OPTIONS ?= -s -w
define EXTRA_GO_LDFLAGS_OPTIONS
-X '"'$(PROJECT_MODULE)/internal/version.Version=$(GIT_VERSION)'"' \
-X '"'$(PROJECT_MODULE)/internal/version.BuildDate=$(BUILD_DATE)'"' \
-X '"'$(PROJECT_MODULE)/internal/version.GitCommit=$(GIT_COMMIT)'"' \
-X '"'$(PROJECT_MODULE)/internal/version.GitBranch=$(GIT_BRANCH)'"' \
-X '"'$(PROJECT_MODULE)/internal/version.BuildUser=$(GIT_USER_EMAIL)'"'
endef

GO_LDFLAGS     := -ldflags "$(GO_LDFLAGS_OPTIONS) $(EXTRA_GO_LDFLAGS_OPTIONS)"
GO_CGO_ENABLED ?= 0
GO_OPTS        ?= -v
GO_OS          ?= linux darwin
GO_ARCH        ?= arm64 amd64
# avoid mocks in tests
GO_FILES       := $(shell go list ./... | grep -v mocks | grep -v docs)
GO_GRAPH_FILE  := $(BUILD_DIR)/go-mod-graph.txt

CONTAINER_NAMESPACE  ?= $(PROJECT_NAMESPACE)
CONTAINER_IMAGE_NAME ?= $(PROJECT_NAME)
# linux ONLY. The release binaries ship for darwin too (GO_OS above), but a
# container image is a Linux artefact: gcr.io/distroless/base has no darwin
# manifest, so `podman build --platform darwin/arm64` fails to pull the base
# and `podman manifest add --os=darwin` fails after it. This carried
# `linux darwin` for a long time and every release "succeeded" with no image
# published: the workflow piped make into tee without pipefail, so the Error
# 125 from both targets was invisible.
CONTAINER_OS         ?= linux
CONTAINER_ARCH       ?= arm64 amd64
# CONTAINER_REPOS     ?= docker.io ghcr.io public.ecr.aws
CONTAINER_REPOS      ?= ghcr.io

CONTAINER_OS_TEST    ?= linux
CONTAINER_ARCH_TEST  ?= amd64

# detect operating system for sed command
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Linux)
	SED_CMD := sed -i
endif
ifeq ($(UNAME_S),Darwin)
	SED_CMD := sed -i .removeit
endif

######## Functions ########
# this is a function that will execute a command and print a message
# MAKE_DEBUG=true make <target> will print the command
# MAKE_STOP_ON_ERRORS=true make any fail will stop the execution if the command fails, this is useful for CI
# NOTE: if the command has a > it will print the output into the original redirect of the command
MAKE_STOP_ON_ERRORS ?= false
MAKE_DEBUG          ?= false

# The Valkey CA the ratelimitvalkey suite needs, when the dev stack has made one.
#
# Those 9 tests connect to Valkey and SKIP when they cannot -- and a skipped test
# reports `ok`, so a suite that verified nothing is indistinguishable from one
# that passed. The dev stack is TLS-only, so with no CA they skip, the package
# measures 17.6% against its 80% floor, and `make test-coverage` fails on a
# package that has nothing wrong with it. That is the state this repo shipped in.
#
# Set automatically rather than documented, because a step you have to remember
# is a step that gets skipped -- and skipping it here is silent. CI has no dev
# certs and gets a plaintext Valkey service container instead; see pr.yaml.
#
# ABSOLUTE, because `go test` runs each package in its own directory.
VALKEY_TEST_CA_FILE := $(CURDIR)/certs/dev/ca.crt
ifneq ($(wildcard $(VALKEY_TEST_CA_FILE)),)
TEST_ENV := VALKEY_TEST_CA=$(VALKEY_TEST_CA_FILE)
endif

define exec_cmd
$(if $(filter $(MAKE_DEBUG),true),\
	${1} \
, \
	$(if $(filter $(MAKE_STOP_ON_ERRORS),true),\
		$(if $(findstring >, $1),\
			@${1} 2>/dev/null && printf "  🤞 ${1} ✅\n" || (printf "  ${1} ❌ 🖕\n"; exit 1) \
		, \
			@${1}  > /dev/null && printf "  🤞 ${1} ✅\n" || (printf "  ${1} ❌ 🖕\n"; exit 1) \
		) \
	, \
		$(if $(findstring >, $1),\
			@${1} 2>/dev/null; _exit_code=$$?; if [ $$_exit_code -eq 0 ]; then printf "  🤞 ${1} ✅\n"; else printf "  ${1} ❌ 🖕\n"; fi; exit $$_exit_code \
		, \
			@${1} > /dev/null 2>&1; _exit_code=$$?; if [ $$_exit_code -eq 0 ]; then printf '  🤞 ${1} ✅\n'; else printf '  ${1} ❌ 🖕\n'; fi; exit $$_exit_code \
		) \
	) \
)

endef # don't remove the white line before endef

# GOVERSION is the toolchain currently building this project, e.g. "go1.27.0".
GOVERSION := $(shell go env GOVERSION)

# ensure_tool,<binary>,<module@version>
#
# Installs a Go tool only when it is missing or was built by an OLDER toolchain
# than the project's. This is not a speed optimisation, it is a correctness one:
# a tool that parses Go source cannot read a stdlib newer than the compiler that
# built it, and the two tools here fail in opposite ways.
#
#   - govulncheck fails LOUDLY: "Int (function) is not a type" out of
#     math/rand/v2 plus a wall of "file requires newer Go version".
#   - betteralign fails SILENTLY: it prints "analysis skipped due to errors in
#     package" for every package and exits 0, which reads exactly like "nothing
#     to align". It went unnoticed until the Go 1.27 bump.
#
# The comparison is "not older than", not "equal to". A tool whose own go.mod
# selects a newer toolchain than ours is fine, and demanding equality there would
# reinstall it on every single invocation.
define ensure_tool
@tool="$$(command -v $(1) 2>/dev/null || true)"; \
	ver="$$([ -n "$$tool" ] && go version "$$tool" 2>/dev/null | awk '{print $$2}' || true)"; \
	if [ -n "$$ver" ] && [ "$$(printf '%s\n%s\n' "$(GOVERSION)" "$$ver" | sort -V | head -n 1)" = "$(GOVERSION)" ]; then \
		printf "  🤞 $(1) is current ($$ver) ✅\n"; \
	else \
		if [ -n "$$ver" ]; then printf "👉 $(1) was built with $$ver, rebuilding with $(GOVERSION)...\n"; \
		else printf "👉 Installing $(1) with $(GOVERSION)...\n"; fi; \
		go install $(2) > /dev/null 2>&1 && printf "  🤞 go install $(2) ✅\n" || { printf "  go install $(2) ❌ 🖕\n"; exit 1; }; \
	fi

endef # don't remove the white line before endef

###############################################################################
######## Targets ##############################################################
##@ Default command
.PHONY: all
all: clean build ## Clean, test and build the application.  Execute by default when make is called without arguments

###############################################################################
##@ Golang commands
.PHONY: go-fmt
go-fmt: ## Format go code
	@printf "👉 Formatting go code...\n"
	$(call exec_cmd, go fmt ./... )

# Deliberately not a dependency of `build`: betteralign rewrites source files,
# and a build target that silently edits the tree is a bad surprise. Run it when
# you have changed a struct.
.PHONY: go-betteralign
go-betteralign: install-betteralign ## Re-align struct fields for optimal memory layout (rewrites source)
	@printf "👉 Aligning struct fields...\n"
	$(call exec_cmd, betteralign -apply ./... )

.PHONY: go-betteralign-check
go-betteralign-check: install-betteralign ## Report struct fields that could be better aligned (no changes)
	@printf "👉 Checking struct field alignment...\n"
	$(call exec_cmd, betteralign ./... )

.PHONY: go-vet
go-vet: ## Vet go code
	@printf "👉 Vet go code...\n"
	$(call exec_cmd, go vet  ./... )

.PHONY: go-generate
go-generate: ## Generate go code
	@printf "👉 Generating go code...\n"
	$(call exec_cmd, go generate ./... )

.PHONY: go-mod-tidy
go-mod-tidy: ## Clean go.mod and go.sum
	@printf "👉 Cleaning go.mod and go.sum...\n"
	$(call exec_cmd, go mod tidy)

.PHONY: go-mod-update
go-mod-update: go-mod-tidy ## Update go.mod and go.sum
	@printf "👉 Updating go.mod and go.sum...\n"
	$(foreach DEP, $(PROJECT_DEPENDENCIES), \
		$(call exec_cmd, go get -u $(DEP)) \
	)
	$(call exec_cmd, go mod tidy)

.PHONY: go-mod-vendor
go-mod-vendor: ## Create mod vendor
	@printf "👉 Creating mod vendor...\n"
	$(call exec_cmd, go mod vendor)

.PHONY: go-mod-verify
go-mod-verify: ## Verify go.mod and go.sum
	@printf "👉 Verifying go.mod and go.sum...\n"
	$(call exec_cmd, go mod verify)

.PHONY: go-mod-download
go-mod-download: ## Download go dependencies
	@printf "👉 Downloading go dependencies...\n"
	$(call exec_cmd, go mod download)

.PHONY: go-mod-graph
go-mod-graph: ## Create a file with the go dependencies graph in build dir
	@printf "👉 Printing go dependencies graph...\n"
	$(call exec_cmd, go mod graph > $(GO_GRAPH_FILE))

# this target is needed to create the dist folder and the coverage file
$(PROJECT_COVERAGE_FILE):
	@printf "👉 Creating coverage file...\n"
	$(call exec_cmd, mkdir -p $(BUILD_DIR) )
	$(call exec_cmd, touch $(PROJECT_COVERAGE_FILE) )

.PHONY: go-test-coverage
go-test-coverage: test ## Shows in you browser the test coverage report per package
	@printf "👉 Running got tool coverage...\n"
	$(call exec_cmd, go tool cover -html=$(PROJECT_COVERAGE_FILE))

###############################################################################
##@ Test commands
.PHONY: test
test: $(PROJECT_COVERAGE_FILE) go-generate go-mod-tidy go-fmt go-vet ## Run tests
	@printf "👉 Running tests...\n"
	$(call exec_cmd, $(TEST_ENV) go test \
		-v -race \
		-coverprofile=$(PROJECT_COVERAGE_FILE) \
		-covermode=$(PROJECT_COVERAGE_MODE) \
		-tags=$(PROJECT_COVERAGE_TAGS) \
		./... \
	)

.PHONY: test-coverage
test-coverage: install-go-test-coverage ## Run tests and show coverage
	@printf "👉 Running tests and showing coverage...\n"
	$(call exec_cmd, go-test-coverage --config=./.testcoverage.yml )

.PHONY: test-integration
test-integration: stop-integration-test go-generate go-mod-tidy go-fmt go-vet start-integration-test ## Run integration tests
	@printf "👉 Running integration tests...\n"
	$(call exec_cmd, go test \
		-v -race \
		-tags=$(PROJECT_INTEGRATION_COVERAGE_TAGS) \
		./tests/integration \
	) \
  make stop-integration-test

.PHONY: test-eval
test-eval: ## Run the RAG retrieval quality gate (needs: make start-dev-env + air + ollama)
	@printf "👉 Running RAG evaluation harness...\n"
	@printf "   Requires the dev stack and Ollama with nomic-embed-text pulled.\n"
	@printf "   The suite skips loudly (never silently) if anything is missing.\n"
	$(call exec_cmd, go test \
		-v -count=1 \
		-tags=eval \
		./tests/eval \
	)

###############################################################################
##@ Build commands
.PHONY: build
build: go-generate go-fmt go-vet docs-swagger  ## Build the API service only
	@printf "👉 Building...\n"
	$(foreach proj_mod, $(PROJECT_MODULES_NAME), \
		$(if $(filter $(PROJECT_NAME),$(proj_mod)), \
			$(call exec_cmd, CGO_ENABLED=$(GO_CGO_ENABLED) go build $(GO_LDFLAGS) $(GO_OPTS) -o $(BUILD_DIR)/$(proj_mod) ./cmd/$(proj_mod)/ ) \
			$(call exec_cmd, chmod +x $(BUILD_DIR)/$(proj_mod) ) \
		) \
	)

.PHONY: build-all
build-all: lint vulncheck go-generate go-fmt go-vet docs-swagger ## Build all the application including the API service and the CLI
	@printf "👉 Building and lintering...\n"
	$(foreach proj_mod, $(PROJECT_MODULES_NAME), \
		$(call exec_cmd, CGO_ENABLED=$(GO_CGO_ENABLED) go build $(GO_LDFLAGS) $(GO_OPTS) -o $(BUILD_DIR)/$(proj_mod) ./cmd/$(proj_mod)/ ) \
		$(call exec_cmd, chmod +x $(BUILD_DIR)/$(proj_mod) ) \
	)

.PHONY: build-dist
build-dist: ## Build the application for all platforms defined in GO_OS and GO_ARCH in this Makefile
	@printf "👉 Building application for different platforms...\n"
	$(foreach GOOS, $(GO_OS), \
		$(foreach GOARCH, $(GO_ARCH), \
			$(foreach proj_mod, $(PROJECT_MODULES_NAME), \
				$(call exec_cmd, GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(GO_CGO_ENABLED) go build $(GO_LDFLAGS) $(GO_OPTS) -o $(DIST_DIR)/$(proj_mod)-$(GOOS)-$(GOARCH) ./cmd/$(proj_mod)/ ) \
				$(call exec_cmd, chmod +x $(DIST_DIR)/$(proj_mod)-$(GOOS)-$(GOARCH)) \
			)\
		)\
	)

.PHONY: build-dist-zip
build-dist-zip: ## Build the application for all platforms defined in GO_OS and GO_ARCH in this Makefile and create a zip file for each binary. Requires make build-dist
	@printf "👉 Creating zip files for distribution...\n"
	$(call exec_cmd, mkdir -p $(DIST_ASSETS_DIR))
	$(foreach GOOS, $(GO_OS), \
		$(foreach GOARCH, $(GO_ARCH), \
			$(foreach proj_mod, $(PROJECT_MODULES_NAME), \
				$(call exec_cmd, zip --junk-paths -r $(DIST_ASSETS_DIR)/$(proj_mod)-$(GOOS)-$(GOARCH).zip $(DIST_DIR)/$(proj_mod)-$(GOOS)-$(GOARCH) ) \
				$(call exec_cmd, shasum -a 256 $(DIST_ASSETS_DIR)/$(proj_mod)-$(GOOS)-$(GOARCH).zip | cut -d ' ' -f 1 > $(DIST_ASSETS_DIR)/$(proj_mod)-$(GOOS)-$(GOARCH).sha256 ) \
			) \
		) \
	)

###############################################################################
##@ Check commands

# podman is what this repo uses locally; CI runners have docker and no podman,
# and the two are argument-compatible for `run --rm -v`. Overridable rather than
# detected, so the failure mode is a clear "command not found" rather than a
# silently different engine.
CONTAINER_ENGINE ?= podman

.PHONY: lint
lint: install-golangci-lint ## Run linters
	@printf "👉 Running linters...\n"
	$(call exec_cmd, golangci-lint run ./...)

.PHONY: vulncheck
vulncheck: install-govulncheck ## Check vulnerabilities
	@printf "👉 Checking vulnerabilities...\n"
	$(call exec_cmd, govulncheck ./...)

.PHONY: check-alerts
check-alerts: ## Validate and unit-test the Prometheus alert rules
	@printf "👉 Checking Prometheus alert rules...\n"
	$(call exec_cmd, $(CONTAINER_ENGINE) run --rm -v $(CURDIR)/dev-env/configuration/prometheus:/rules:ro -w /rules --entrypoint promtool docker.io/prom/prometheus:latest check rules alerts.yaml)
	$(call exec_cmd, $(CONTAINER_ENGINE) run --rm -v $(CURDIR)/dev-env/configuration/prometheus:/rules:ro -w /rules --entrypoint promtool docker.io/prom/prometheus:latest test rules alerts_test.yaml)

.PHONY: arch-test
arch-test: ## Run the hexagonal architecture test (forbids infra imports under internal/core/...)
	@printf "👉 Running architecture test (hexagonal invariant)...\n"
	$(call exec_cmd, go test -count=1 -run TestCoreHasNoInfraImports ./internal/core/)

.PHONY: licenses
licenses: install-go-licenses ## Generate third-party license reports (CSV) for every binary in cmd/
	@printf "👉 Generating third-party license reports...\n"
	$(call exec_cmd, mkdir -p $(LICENSES_DIR))
	$(foreach proj_mod, $(PROJECT_MODULES_NAME), \
		$(call exec_cmd, go-licenses csv --ignore $(LICENSES_IGNORE) $(PROJECT_MODULE)/cmd/$(proj_mod) > $(LICENSES_DIR)/$(proj_mod)-third-party-licenses.csv) \
	)

.PHONY: licenses-check
licenses-check: install-go-licenses ## Verify third-party dependency licenses are allowed for every binary in cmd/
	@printf "👉 Checking third-party licenses...\n"
	$(foreach proj_mod, $(PROJECT_MODULES_NAME), \
		$(call exec_cmd, go-licenses check --ignore $(LICENSES_IGNORE) $(PROJECT_MODULE)/cmd/$(proj_mod)) \
	)

# this is necessary to avoid a comma in the call function
COMMA_SIGN := ,
###############################################################################
##@ Docs commands
.PHONY: docs-swagger
docs-swagger: install-swag install-go-swagger ## Generate swagger documentation
	@printf "👉 Generating swagger documentation...\n"
	$(foreach proj_mod, $(PROJECT_MODULES_NAME), \
		$(if $(filter $(PROJECT_NAME),$(proj_mod)), \
			$(call exec_cmd, swag fmt \
				--dir ./cmd/$(proj_mod)$(COMMA_SIGN)./internal \
			) \
			$(call exec_cmd, swag init \
				--dir ./cmd/$(proj_mod)$(COMMA_SIGN)./internal/adapter/driving/http/handler \
				--output ./docs/api \
				--parseDependency true \
				--parseInternal true \
			) \
			$(call exec_cmd, swagger generate markdown \
				--spec ./docs/api/swagger.json \
				--target ./docs/api/  \
			) \
		) \
	)

.PHONY: docs-api-resources
docs-api-resources: docs-swagger ## Generate the authz resource rows from swagger.json, for 00008_roles_policies_tables_upsert.sql
	@printf "👉 Generating the authz resource rows from ./docs/api/swagger.json...\n"
	@printf "   Paste the block below over the generated rows in\n"
	@printf "   database/migrations/00008_roles_policies_tables_upsert.sql, after the\n"
	@printf "   '-- automatic generate with the program apiendpoints' marker.\n"
	@printf "   A new endpoint also needs its row added by a NEW migration: an existing\n"
	@printf "   database will not re-run 00008.\n\n"
	@go run ./cmd/apiendpoints/main.go

###############################################################################
##@ Tools commands
.PHONY: install-air
install-air: ## Install air for hot reload (https://github.com/cosmtrek/air)
	$(call ensure_tool,air,github.com/air-verse/air@latest)

.PHONY: install-swag
install-swag: ## Install swag for swagger documentation (https://github.com/swaggo/http-swagger)
	$(call ensure_tool,swag,github.com/swaggo/swag/cmd/swag@latest)

.PHONY: install-go-swagger
install-go-swagger: ## Install go-swagger, used to render the API reference as markdown (https://goswagger.io/)
	$(call ensure_tool,swagger,github.com/go-swagger/go-swagger/cmd/swagger@latest)

.PHONY: install-goose
install-goose: ## Install goose for database migrations (https://github.com/pressly/goose)
	$(call ensure_tool,goose,github.com/pressly/goose/v3/cmd/goose@latest)

.PHONY: install-go-test-coverage
install-go-test-coverage: ## Install got tool for test coverage (https://github.com/vladopajic/go-test-coverage)
	$(call ensure_tool,go-test-coverage,github.com/vladopajic/go-test-coverage/v2@latest)

.PHONY: install-govulncheck
install-govulncheck: ## Install govulncheck for vulnerabilities check (https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck#section-documentation)
	$(call ensure_tool,govulncheck,golang.org/x/vuln/cmd/govulncheck@latest)

.PHONY: install-go-licenses
install-go-licenses: ## Install go-licenses for third-party license reporting (https://github.com/google/go-licenses)
	$(call ensure_tool,go-licenses,github.com/google/go-licenses@latest)

.PHONY: install-golangci-lint
install-golangci-lint: ## Install golangci-lint for linting (https://golangci-lint.run/)
	$(call ensure_tool,golangci-lint,github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2)

.PHONY: install-betteralign
install-betteralign: ## Install betteralign for struct field alignment (https://github.com/dkorunic/betteralign)
	$(call ensure_tool,betteralign,github.com/dkorunic/betteralign/cmd/betteralign@latest)

.PHONY: tools
tools: install-air install-swag install-go-swagger install-goose install-go-test-coverage install-govulncheck install-go-licenses install-golangci-lint install-betteralign ## Install or rebuild every Go tool this repo uses
	@printf "👉 All tools present and built with $(GOVERSION)\n"

###############################################################################
##@ Development commands
.PHONY: check-dev-env-tools
check-dev-env-tools: ## Verify the tools the dev-env targets need are on PATH (openssl, envsubst)
	@for tool in $(DEV_ENV_EXECUTABLES); do \
		command -v $$tool >/dev/null 2>&1 || { \
			printf "  $$tool not found in PATH ❌ -- on macOS: brew install gettext openssl; on Debian/Ubuntu: apt install gettext-base openssl\n"; exit 1; }; \
	done

# Everything under certs/ that the dev stack needs, generated only when missing:
#
#   certs/jwt.key, certs/jwt.pub              signs and verifies every token
#   certs/aes-256-symmetric-hex.key           encrypts IdP client secrets at rest
#   certs/dev/{ca.crt,server.crt,server.key}  TLS for PostgreSQL and Valkey
#
# Never overwrites. A new jwt.key invalidates every token the running service
# has issued; a new AES key makes every secret already encrypted with the old
# one unreadable; a new CA is not trusted by the service that already loaded
# the old one. To rotate on purpose, delete the file and run this again.
.PHONY: dev-certs
# The host the HTTP server's dev certificate is issued for. It has to match the
# filename .air.toml and run.sh pass to http.server.tls.cert.file.
DEV_TLS_HOST ?= goapitemplate.local

dev-certs: check-dev-env-tools ## Generate the JWT pair, the AES key, the HTTP server's TLS pair and the dev TLS CA under certs/ (idempotent, never overwrites)
	@printf "👉 Generating development key material under certs/...\n"
		$(call exec_cmd, ./dev-env/scripts/generate-dev-keys.sh $(CURDIR)/certs $(DEV_TLS_HOST) )
		$(call exec_cmd, ./dev-env/scripts/generate-dev-certs.sh $(CURDIR)/certs/dev )
	@printf "  certs/: %s\n" "$$(cd certs && ls -1 | tr '\n' ' ')"
	@printf "  certs/dev/: %s\n" "$$(cd certs/dev && ls -1 | tr '\n' ' ')"

.PHONY: stop-dev-env
stop-dev-env: check-dev-env-tools ## Stop the development environment pod (PostgreSQL, Valkey, Grafana, Tempo, Prometheus, Mailpit). Keeps the data
	@printf "👉 Stopping the development environment...\n"
		$(call exec_cmd, PROJECT_NAME=$(PROJECT_NAME) CERTS_DIR=$(CURDIR)/certs/dev envsubst < ./dev-env/provisioning/dev-service-pod.yaml.tmpl > ./dev-env/provisioning/dev-service-pod.yaml)
		$(call exec_cmd, podman play kube --down ./dev-env/provisioning/dev-service-pod.yaml )

.PHONY: start-dev-env
start-dev-env: stop-dev-env dev-certs install-air install-swag install-goose ## Start the development environment pod. WARNING: stops the running one and RECREATES its data
	@printf "👉 Starting the development environment...\n"
		$(call exec_cmd, PROJECT_NAME=$(PROJECT_NAME) CERTS_DIR=$(CURDIR)/certs/dev envsubst < ./dev-env/provisioning/dev-service-pod.yaml.tmpl > ./dev-env/provisioning/dev-service-pod.yaml)
		$(call exec_cmd, mkdir -p $(HOME)/tmp/$(PROJECT_NAME)/db-volume-host )
		$(call exec_cmd, mkdir -p $(HOME)/tmp/$(PROJECT_NAME)/tempo-volume-host )
		$(call exec_cmd, mkdir -p $(HOME)/tmp/$(PROJECT_NAME)/prometheus-volume-host )
		$(call exec_cmd, mkdir -p $(HOME)/tmp/$(PROJECT_NAME)/grafana-configuration )
		$(call exec_cmd, mkdir -p $(HOME)/tmp/$(PROJECT_NAME)/grafana-ds )
		$(call exec_cmd, mkdir -p $(HOME)/tmp/$(PROJECT_NAME)/grafana-dashboard-config )
		$(call exec_cmd, mkdir -p $(HOME)/tmp/$(PROJECT_NAME)/grafana-dashboard )
		$(call exec_cmd, mkdir -p $(HOME)/tmp/$(PROJECT_NAME)/dev-env)
		$(call exec_cmd, chmod 777 $(HOME)/tmp/$(PROJECT_NAME)/tempo-volume-host )
		$(call exec_cmd, chmod 777 $(HOME)/tmp/$(PROJECT_NAME)/prometheus-volume-host )

		$(call exec_cmd, cp ./dev-env/configuration/grafana/configuration/grafana.ini $(HOME)/tmp/$(PROJECT_NAME)/grafana-configuration/grafana.ini)
		$(call exec_cmd, cp ./dev-env/configuration/grafana/datasource/grafana-ds.yaml $(HOME)/tmp/$(PROJECT_NAME)/grafana-ds/grafana-ds.yaml)
		$(call exec_cmd, cp ./dev-env/configuration/grafana/dashboard/default.yaml $(HOME)/tmp/$(PROJECT_NAME)/grafana-dashboard-config/default.yaml)
		$(call exec_cmd, cp ./dev-env/configuration/grafana/dashboard/*.json $(HOME)/tmp/$(PROJECT_NAME)/grafana-dashboard/)
		$(call exec_cmd, cp ./dev-env/configuration/prometheus/prometheus.yaml $(HOME)/tmp/$(PROJECT_NAME)/dev-env/prometheus.yaml )
		$(call exec_cmd, cp ./dev-env/configuration/prometheus/alerts.yaml $(HOME)/tmp/$(PROJECT_NAME)/dev-env/alerts.yaml )
		$(call exec_cmd, cp ./dev-env/configuration/tempo/tempo-local-config.yaml $(HOME)/tmp/$(PROJECT_NAME)/dev-env/tempo-local-config.yaml )

		$(call exec_cmd, podman play kube ./dev-env/provisioning/dev-service-pod.yaml )
	@printf "👉 Development environment is up. Data lives under $(HOME)/tmp/$(PROJECT_NAME). Next: air\n"

.PHONY: rm-dev-env
rm-dev-env: stop-dev-env  ## Stop the development environment pod and delete its data under $HOME/tmp/<project>
	@printf "👉 Removing development environment...\n"
		$(call exec_cmd, rm -rf $(HOME)/tmp/$(PROJECT_NAME) 2>/dev/null )

# Three substitutions, longest first, so each one leaves the next its input:
#
#   slashdevops/go-rest-api-service-template  -> <owner>/<name>   module path, badges, image name
#   go-rest-api-service-template              -> <name>           binary, pod, database, docs
#   go_rest_api_service_template              -> <name>           identifiers that cannot carry a dash
#
# Only the FULL owner/name pair is replaced, never the bare owner: go.mod also
# requires github.com/slashdevops/{c3e,httpx,mailer,...}, which must stay.
#
# .git is excluded because rewriting packed refs and logs corrupts the clone;
# build, dist and certs are generated or secret and are not sources.
#
# `--null`, not `-Z`: on BSD grep (macOS) `-Z` means "decompress", so the file
# list reached xargs as one newline-joined name and every sed failed with
# "File name too long" -- while the target still reported success.
RENAME_GREP := grep -rlI --null --exclude-dir=.git --exclude-dir=build --exclude-dir=dist --exclude-dir=certs --exclude-dir=node_modules

.PHONY: rename-project
rename-project: clean ## Rename the project after creating a repository from the template (owner/name come from the origin remote)
	@printf "👉 Renaming project...\n"
	$(if $(or $(findstring /,$(GIT_REPOSITORY_OWNER)$(GIT_REPOSITORY_NAME)),$(findstring :,$(GIT_REPOSITORY_OWNER)$(GIT_REPOSITORY_NAME)),$(findstring @,$(GIT_REPOSITORY_OWNER)$(GIT_REPOSITORY_NAME)),$(filter-out 1,$(words $(GIT_REPOSITORY_OWNER))),$(filter-out 1,$(words $(GIT_REPOSITORY_NAME)))), \
		$(error could not read the repository owner and name from the origin remote ("$(GIT_REPOSITORY_URL)"). Set them by hand: GIT_REPOSITORY_OWNER=<owner> GIT_REPOSITORY_NAME=<name> make rename-project))
	@set -e; if [ "$(GIT_REPOSITORY_OWNER)/$(GIT_REPOSITORY_NAME)" = "$(TEMPLATE_REPOSITORY)" ]; then \
		printf "  the project is already named $(TEMPLATE_REPOSITORY) ✅\n"; \
	else \
		$(RENAME_GREP) "$(TEMPLATE_REPOSITORY)" . | xargs -0 -r $(SED_CMD) "s|$(TEMPLATE_REPOSITORY)|$(GIT_REPOSITORY_OWNER)/$(GIT_REPOSITORY_NAME)|g"; \
		printf "  🤞 $(TEMPLATE_REPOSITORY) -> $(GIT_REPOSITORY_OWNER)/$(GIT_REPOSITORY_NAME) ✅\n"; \
		$(RENAME_GREP) "$(TEMPLATE_NAME)" . | xargs -0 -r $(SED_CMD) "s|$(TEMPLATE_NAME)|$(GIT_REPOSITORY_NAME)|g"; \
		printf "  🤞 $(TEMPLATE_NAME) -> $(GIT_REPOSITORY_NAME) ✅\n"; \
		$(RENAME_GREP) "$(TEMPLATE_NAME_UNDERSCORE)" . | xargs -0 -r $(SED_CMD) "s|$(TEMPLATE_NAME_UNDERSCORE)|$(GIT_REPOSITORY_NAME_UNDERSCORE)|g"; \
		printf "  🤞 $(TEMPLATE_NAME_UNDERSCORE) -> $(GIT_REPOSITORY_NAME_UNDERSCORE) ✅\n"; \
		find . -name "*.removeit" -not -path "./.git/*" -exec rm -f {} +; \
		if [ -d "cmd/$(TEMPLATE_NAME)" ]; then \
			mv "cmd/$(TEMPLATE_NAME)" "cmd/$(GIT_REPOSITORY_NAME)"; \
			printf "  🤞 cmd/$(TEMPLATE_NAME) -> cmd/$(GIT_REPOSITORY_NAME) ✅\n"; \
		fi; \
		go mod tidy; \
		printf "  🤞 go mod tidy ✅\n"; \
	fi
	@printf "👉 Project is $(GIT_REPOSITORY_OWNER)/$(GIT_REPOSITORY_NAME). Review with 'git diff', commit, then: make tools && make dev-certs && make start-dev-env\n"

.PHONY: start-integration-test
start-integration-test:rm-dev-env stop-integration-test container-build-integration-test ## Start the integration test
	@printf "👉 Starting integration test...\n"
	$(call exec_cmd, podman play kube ./tests/provisioning/integration-test.yaml )

.PHONY: stop-integration-test
stop-integration-test: ## Stop the integration test
	@printf "👉 Stopping integration test...\n"
	$(call exec_cmd, podman play kube --down ./tests/provisioning/integration-test.yaml )
	$(call exec_cmd, podman rm -f integration-test )

###############################################################################
##@ Container commands
.PHONY: container-build-integration-test
container-build-integration-test: build-dist ## Build the container image, requires make build-dist
	@printf "👉 Building container images for integration test...\n"
		$(call exec_cmd, podman build \
			--platform $(CONTAINER_OS_TEST)/$(CONTAINER_ARCH_TEST) \
			--tag $(CONTAINER_NAMESPACE)/$(CONTAINER_IMAGE_NAME):test-integration \
			--build-arg SERVICE_NAME=$(PROJECT_NAME) \
			--build-arg GOOS=$(CONTAINER_OS_TEST) \
			--build-arg GOARCH=$(CONTAINER_ARCH_TEST) \
			--build-arg BUILD_DATE=$(BUILD_DATE) \
			--build-arg BUILD_VERSION=$(GIT_VERSION) \
			--build-arg DESCRIPTION="Container image for $(PROJECT_NAME)" \
			--build-arg REPO_URL="https://github.com/$(PROJECT_NAMESPACE)/$(PROJECT_NAME)" \
			--file ./tests/provisioning/Containerfile . \
	)

.PHONY: container-build
container-build: ## Build the container image, requires make build-dist
	@printf "👉 Building container images...\n"
	$(foreach OS, $(CONTAINER_OS), \
		$(foreach ARCH, $(CONTAINER_ARCH), \
			$(if $(findstring v, $(ARCH)), $(eval BIN_ARCH = arm64), $(eval BIN_ARCH = $(ARCH)) ) \
			$(call exec_cmd, podman build \
				--platform $(OS)/$(BIN_ARCH) \
				--tag $(CONTAINER_NAMESPACE)/$(CONTAINER_IMAGE_NAME):$(GIT_VERSION)-$(OS)-$(ARCH) \
				--build-arg SERVICE_NAME=$(PROJECT_NAME) \
				--build-arg GOOS=$(OS) \
				--build-arg GOARCH=$(ARCH) \
				--build-arg BUILD_DATE=$(BUILD_DATE) \
				--build-arg BUILD_VERSION=$(GIT_VERSION) \
				--build-arg DESCRIPTION="Container image for $(PROJECT_NAME)" \
				--build-arg REPO_URL="https://github.com/$(PROJECT_NAMESPACE)/$(PROJECT_NAME)" \
				--file ./Containerfile . \
			) \
		) \
	)

.PHONY: container-login
container-login: ## Login to the container registry. Requires REPOSITORY_REGISTRY_TOKEN and REPOSITORY_REGISTRY_USERNAME env var
	@printf "👉 Logging in to container registry...\n"
	$(foreach REPO, $(CONTAINER_REPOS), \
		$(call exec_cmd, echo $(REPOSITORY_REGISTRY_TOKEN) | podman login $(REPO) --username $(REPOSITORY_REGISTRY_USERNAME) --password-stdin ) \
	)

.PHONY: container-publish
container-publish: ## Publish the container image to the container registry
	@printf "👉 Creating container manifest...\n"
	$(foreach REPO, $(CONTAINER_REPOS), \
		$(if $(shell podman manifest exists $(REPO)/$(CONTAINER_NAMESPACE)/$(CONTAINER_IMAGE_NAME):$(GIT_VERSION) || echo "exists" ), \
		, \
			$(call exec_cmd, podman manifest rm $(REPO)/$(CONTAINER_NAMESPACE)/$(CONTAINER_IMAGE_NAME):$(GIT_VERSION) ) \
		) \
		$(call exec_cmd, podman manifest create $(REPO)/$(CONTAINER_NAMESPACE)/$(CONTAINER_IMAGE_NAME):$(GIT_VERSION) \
		) \
		$(foreach OS, $(CONTAINER_OS), \
			$(foreach ARCH, $(CONTAINER_ARCH), \
				$(if $(findstring v, $(ARCH)), $(eval BIN_ARCH = arm64), $(eval BIN_ARCH = $(ARCH)) ) \
				$(call exec_cmd, podman manifest add --os=$(OS) --arch=$(ARCH) \
					$(REPO)/$(CONTAINER_NAMESPACE)/$(CONTAINER_IMAGE_NAME):$(GIT_VERSION) \
					containers-storage:localhost/$(CONTAINER_NAMESPACE)/$(CONTAINER_IMAGE_NAME):$(GIT_VERSION)-$(OS)-$(ARCH) \
				) \
			) \
		) \
	)

	@printf "👉 Publishing container images...\n"
	$(foreach REPO, $(CONTAINER_REPOS), \
		$(call exec_cmd, podman manifest push --all \
			$(REPO)/$(CONTAINER_NAMESPACE)/$(CONTAINER_IMAGE_NAME):$(GIT_VERSION) \
			docker://$(REPO)/$(CONTAINER_NAMESPACE)/$(CONTAINER_IMAGE_NAME):$(GIT_VERSION) ) \
		$(call exec_cmd, podman manifest push --all \
			$(REPO)/$(CONTAINER_NAMESPACE)/$(CONTAINER_IMAGE_NAME):$(GIT_VERSION) \
			docker://$(REPO)/$(CONTAINER_NAMESPACE)/$(CONTAINER_IMAGE_NAME):latest ) \
	)

###############################################################################
##@ Support Commands
.PHONY: clean
clean: ## Clean the environment
	@printf "👉 Cleaning environment...\n"
	$(call exec_cmd, go clean -n -x -i)
	$(call exec_cmd, rm -rf $(BUILD_DIR) $(DIST_DIR) )

# Test target to verify error handling
.PHONY: test-fail
test-fail: ## Test target that always fails (for testing error handling)
	@printf "👉 Testing error handling...\n"
	$(call exec_cmd, false)  # 'false' command always returns exit code 1
	@printf "This should not be printed if MAKE_STOP_ON_ERRORS=true\n"

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##";                                             \
		printf "Usage: make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ \
		{ printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2 } /^##@/            \
		{ printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } '                  \
		$(MAKEFILE_LIST)

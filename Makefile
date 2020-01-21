# --------------------------------------------------------------------------------------------------
# Variables
# --------------------------------------------------------------------------------------------------
-include app.mk

GIT_SUMMARY := $(shell git describe --tags --dirty --always)
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD)
BUILD_STAMP := $(shell date -u '+%Y-%m-%dT%H:%M:%S%z')
GOLANGCI_LINT_CONFIG_URL := https://raw.githubusercontent.com/utilitywarehouse/partner-go-build/master/.golangci.yml
GOLANGCI_LINT_CONFIG_PATH := .golangci.yml

LDFLAGS := -ldflags ' \
	-X "github.com/utilitywarehouse/partner-mono/pkg/meta.ApplicationName=$(APP_NAME)" \
	-X "github.com/utilitywarehouse/partner-mono/pkg/meta.ApplicationDescription=$(APP_DESCRIPTION)" \
	-X "github.com/utilitywarehouse/partner-mono/pkg/meta.GitSummary=$(GIT_SUMMARY)" \
	-X "github.com/utilitywarehouse/partner-mono/pkg/meta.GitBranch=$(GIT_BRANCH)" \
	-X "github.com/utilitywarehouse/partner-mono/pkg/meta.BuildStamp=$(BUILD_STAMP)"'

$(shell cp -n .env.example .env)

.info: ## show project build metadata
	@echo APP_NAME: $(APP_NAME)
	@echo APP_DESCRIPTION: $(APP_DESCRIPTION)
	@echo GIT_SUMMARY: $(GIT_SUMMARY)
	@echo GIT_BRANCH: $(GIT_BRANCH)
	@echo BUILD_STAMP: $(BUILD_STAMP)
	@echo LDFLAGS: $(LDFLAGS)

.help: ## show help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-40s\033[0m %s\n", $$1, $$2}'

# --------------------------------------------------------------------------------------------------
# Application Tasks
# --------------------------------------------------------------------------------------------------

fast: ## do a fast build
	go build $(LDFLAGS)

ensure-protos: # placeholder

.PHONY: test
test: ## run tests on package and all subpackages
	docker-compose up -d
	go test $(LDFLAGS) -v -race ./...

lint: ## run the linter
	curl -o $(GOLANGCI_LINT_CONFIG_PATH) -s $(GOLANGCI_LINT_CONFIG_URL) && \
	golangci-lint run --deadline=2m --config=$(GOLANGCI_LINT_CONFIG_PATH)

test-all: lint

clean: ## clean the build and test caches
	go clean -testcache ./...

# --------------------------------------------------------------------------------------------------
# Housekeeping - run this to update the metafiles/dotfiles/etc
# --------------------------------------------------------------------------------------------------

housekeeping: ## automatically update metafiles (Makefile.common.mk, .circleci/config.yml)
	-rm -rf .tmp
	mkdir .tmp
	cd .tmp && \
	git init && \
	git config core.sparsecheckout true && \
	echo repositories/go/ >> .git/info/sparse-checkout && \
	git remote add origin git@github.com:utilitywarehouse/partner && \
	git pull --depth=1 origin master

	-cp -n .tmp/repositories/go/app.mk app.mk
	cp -r .tmp/repositories/go/.circleci/ .circleci/
	cp .tmp/repositories/go/Makefile.common.mk Makefile.common.mk

	-rm -rf .tmp

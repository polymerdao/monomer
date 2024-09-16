GOBIN ?= $$(go env GOPATH)/bin
COVER_OUT ?= cover.out
COVER_HTML ?= cover.html
SCRIPTS_PATH ?= scripts

E2E_ARTIFACTS_PATH ?= e2e/artifacts
E2E_STATE_SETUP_PATH ?= e2e/optimism/.devnet
E2E_CONFIG_SETUP_PATH ?= e2e/optimism/packages/contracts-bedrock/deploy-config/devnetL1.json
FOUNDRY_ARTIFACTS_PATH ?= bindings/artifacts
FOUNDRY_CACHE_PATH ?= bindings/cache

.PHONY: test
test:
	go test -short ./...

.PHONY: test-all
test-all:
	go test ./...

.PHONY: e2e
e2e:
	go test -v ./e2e

.PHONY: install-golangci-lint
install-golangci-lint:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.60.1

.PHONY: lint
lint:
	${GOBIN}/golangci-lint run
	${GOBIN}/buf lint

.PHONY: gen-proto
gen-proto:
	${SCRIPTS_PATH}/gen-proto.sh

.PHONY: install-gofumpt
install-gofumpt:
	go install mvdan.cc/gofumpt@v0.6.0

.PHONY: install-buf
install-buf:
	go install github.com/bufbuild/buf/cmd/buf@v1.32.0
	go install github.com/cosmos/gogoproto/protoc-gen-gocosmos@v1.5.0

.PHONY: install-go-test-coverage
install-go-test-coverage:
	go install github.com/vladopajic/go-test-coverage/v2@v2.9.0

.PHONY: install-abi-gen
install-abi-gen:
	go install github.com/ethereum/go-ethereum/cmd/abigen@v1.10.25

.PHONY: install-mockgen
install-mockgen:
	go install github.com/golang/mock/mockgen@v1.6.0

.PHONY: install-foundry
install-foundry:
	${SCRIPTS_PATH}/install-foundry.sh

.PHONY: gen-bindings
gen-bindings:
	${SCRIPTS_PATH}/generate-bindings.sh

.PHONY: gen-mocks
gen-mocks:
	mockgen -source=x/rollup/types/expected_keepers.go -package testutil -destination x/rollup/testutil/expected_keepers_mocks.go

$(COVER_OUT):
	go test -short ./... -coverprofile=$@ -covermode=atomic -coverpkg=./...

.PHONY: check-cover
check-cover: $(COVER_OUT)
	${GOBIN}/go-test-coverage --config=./.testcoverage.yml

$(COVER_HTML): $(COVER_OUT)
	go tool cover -html=$< -o $@

.PHONY: clean
clean:
	if [ -f $(COVER_OUT) ]; then rm $(COVER_OUT); fi
	if [ -f $(COVER_HTML) ]; then rm $(COVER_HTML); fi
	if [ -d ${E2E_ARTIFACTS_PATH} ]; then rm -r ${E2E_ARTIFACTS_PATH}; fi
	if [ -d ${E2E_STATE_SETUP_PATH} ]; then rm -r ${E2E_STATE_SETUP_PATH}; fi
	if [ -f $(E2E_CONFIG_SETUP_PATH) ]; then rm $(E2E_CONFIG_SETUP_PATH); fi
	if [ -d ${FOUNDRY_ARTIFACTS_PATH} ]; then rm -r ${FOUNDRY_ARTIFACTS_PATH}; fi
	if [ -d ${FOUNDRY_CACHE_PATH} ]; then rm -r ${FOUNDRY_CACHE_PATH}; fi

.PHONY: setup-e2e
setup-e2e:
	$(MAKE) -C e2e/optimism install-geth && \
	$(MAKE) -C e2e/optimism cannon-prestate && \
	$(MAKE) -C e2e/optimism devnet-allocs && \
	# Make genesis state and config available to op-e2e/config/init.go. See issue #207. && \
	cp -r e2e/optimism/.devnet/ ./.devnet && \
	mkdir -p ./packages/contracts-bedrock/deploy-config && \
	cp ./e2e/optimism/packages/contracts-bedrock/deploy-config/devnetL1.json ./packages/contracts-bedrock/deploy-config/devnetL1.json

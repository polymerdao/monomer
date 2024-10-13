GOBIN ?= $$(go env GOPATH)/bin
COVER_OUT ?= cover.out
COVER_HTML ?= cover.html
SCRIPTS_PATH ?= scripts
BIN ?= bin
GO_WRAPPER ?= $(SCRIPTS_PATH)/go-wrapper.sh

E2E_ARTIFACTS_PATH ?= e2e/artifacts
E2E_STATE_SETUP_PATH ?= e2e/optimism/.devnet
E2E_CONFIG_SETUP_PATH ?= e2e/optimism/packages/contracts-bedrock/deploy-config/devnetL1.json
FOUNDRY_ARTIFACTS_PATH ?= bindings/artifacts
FOUNDRY_CACHE_PATH ?= bindings/cache

.PHONY: monogen
monogen:
	go build -o $(BIN)/monogen ./monogen/cmd

.PHONY: testapp
testapp:
	go build -o $(BIN)/testappd ./monogen/testapp/cmd/testappd/

.PHONY: test
test:
	$(GO_WRAPPER) test -short ./...

.PHONY: test-all
test-all:
	$(GO_WRAPPER) test ./...

.PHONY: e2e
e2e:
	$(GO_WRAPPER) test -v ./e2e \
	-l1-allocs ./optimism/.devnet/allocs-l1.json \
	-l2-allocs-dir ./optimism/.devnet/ \
	-l1-deployments ./optimism/.devnet/addresses.json \
	-deploy-config ./optimism/packages/contracts-bedrock/deploy-config/devnetL1.json

.PHONY: wallet-integration
wallet-integration:
	go run github.com/eliben/static-server@v1.3.0 -port=0 opdevnet/wallet

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
	go install go.uber.org/mock/mockgen@v0.4.0

.PHONY: install-foundry
install-foundry:
	${SCRIPTS_PATH}/install-foundry.sh

.PHONY: gen-bindings
gen-bindings:
	${SCRIPTS_PATH}/generate-bindings.sh

.PHONY: gen-mocks
gen-mocks:
	mockgen -source=x/rollup/types/expected_keepers.go -package testutil -destination x/rollup/testutil/expected_keepers_mocks.go

.PHONY: zip-testapp
zip-testapp:
	$(MAKE) -C monogen zip-testapp

$(COVER_OUT):
	$(GO_WRAPPER) test -short ./... -coverprofile=$@ -covermode=atomic -coverpkg=./...

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
	if [ -d $(BIN) ]; then rm -r $(BIN); fi

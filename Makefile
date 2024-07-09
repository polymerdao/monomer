GOBIN ?= $$(go env GOPATH)/bin
COVER_OUT ?= cover.out
COVER_HTML ?= cover.html
PULSAR_PATHS ?= proto/rollup/module,testapp/proto/testapp/module

E2E_ARTIFACTS_PATH ?= e2e/artifacts
E2E_STATE_SETUP_PATH ?= e2e/optimism/.devnet
E2E_CONFIG_SETUP_PATH ?= e2e/optimism/packages/contracts-bedrock/deploy-config/devnetL1.json

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
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.57.2

.PHONY: lint
lint:
	${GOBIN}/golangci-lint run
	${GOBIN}/buf lint

.PHONY: gen-proto
gen-proto:
	${GOBIN}/buf generate --template buf.gen.gocosmos.yaml --exclude-path $(PULSAR_PATHS)
	${GOBIN}/buf generate --template buf.gen.pulsar.yaml --path $(PULSAR_PATHS)

.PHONY: install-gofumpt
install-gofumpt:
	go install mvdan.cc/gofumpt@v0.6.0

.PHONY: install-buf
install-buf:
	go install github.com/bufbuild/buf/cmd/buf@v1.31.0

.PHONY: install-go-test-coverage
install-go-test-coverage:
	go install github.com/vladopajic/go-test-coverage/v2@v2.9.0

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

.PHONY: setup-e2e
setup-e2e:
	$(MAKE) -C e2e/optimism install-geth && \
		$(MAKE) -C e2e/optimism cannon-prestate && \
		$(MAKE) -C e2e/optimism devnet-allocs

GOBIN ?= $$(go env GOPATH)/bin
BINARY_NAME ?= monomer
COVER_OUT ?= cover.out
COVER_HTML ?= cover.html

.PHONY: $(BINARY_NAME)
$(BINARY_NAME):
	go build -o $(BINARY_NAME) ./cmd/monomer/

.PHONY: install-golangci-lint
install-golangci-lint:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.56.2

.PHONY: lint
lint: install-golangci-lint
	${GOBIN}/golangci-lint run

.PHONY: install-go-test-coverage
install-go-test-coverage:
	go install github.com/vladopajic/go-test-coverage/v2@v2.9.0

$(COVER_OUT):
	go test ./... -coverprofile=$@ -covermode=atomic -coverpkg=./...
	${GOBIN}/go-test-coverage --config=./.testcoverage.yml

$(COVER_HTML): $(COVER_OUT)
	go tool cover -html=$< -o $@

.PHONY: test
test: 
	go test ./...

.PHONY: clean
clean:
	if [ -f $(BINARY_NAME) ]; then rm $(BINARY_NAME); fi
	if [ -f $(COVER_OUT) ]; then rm $(COVER_OUT); fi
	if [ -f $(COVER_HTML) ]; then rm $(COVER_HTML); fi

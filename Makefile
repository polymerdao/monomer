GOBIN ?= $$(go env GOPATH)/bin
COVER_OUT ?= cover.out
COVER_HTML ?= cover.html

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

.PHONY: install-gofumpt
install-gofumpt:
	go install mvdan.cc/gofumpt@v0.6.0

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
	if ls *.log 1> /dev/null 2>&1; then rm *.log; fi

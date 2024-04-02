.PHONY: monomer
monomer:
	@go build -o monomer ./cmd/monomer/

.PHONY: clean
clean:
	@rm ./monomer

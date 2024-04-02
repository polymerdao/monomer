.PHONY: monomer
monomer:
	@go build -o monomer ./cmd/monomer/

.PHONY: clean
clean:
	@if [ -f ./monomer ]; then rm ./monomer; fi

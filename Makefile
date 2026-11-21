GO ?= go
BINARY := bin/sheriff

.PHONY: build test vet clean

build:
	$(GO) build -o $(BINARY) ./cmd/sheriff

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

clean:
	rm -rf bin

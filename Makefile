.PHONY: build test vet lint install
build:
	go build -trimpath -o bin/heimdall ./cmd/heimdall
test:
	go test ./...
vet:
	go vet ./...
lint: vet
install:
	install -d $(HOME)/.local/bin
	go build -trimpath -o $(HOME)/.local/bin/heimdall ./cmd/heimdall

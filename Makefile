.PHONY: fmt test vet build check clean

BINARY ?= gogitor

fmt:
	gofmt -w $$(find . -name '*.go' -type f)

test:
	go test ./...

vet:
	go vet ./...

build:
	go build -trimpath -ldflags='-s -w' -o $(BINARY) ./cmd/gogitor/

check: fmt test vet build

clean:
	rm -f $(BINARY)

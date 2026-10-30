# GenePath Runner — developer shortcuts.
#
# Everything here is optional: the README also lists the plain `go` commands,
# which is what CI runs. Use `make` if you have it, type the commands if you do
# not.

BINARY := bin/genepath
VERSION ?= 0.4.0
DB ?= data/genepath.db
ADDR ?= 127.0.0.1:8080

.PHONY: build run test cover vet fmt docker docker-run clean

build:
	go build -trimpath -ldflags="-s -w" -o $(BINARY) ./cmd/server

run: build
	$(BINARY) -addr $(ADDR) -db $(DB) -version $(VERSION)

test:
	go test ./... -count=1

cover:
	go test ./... -count=1 -coverprofile=coverage.out -covermode=atomic
	go tool cover -func=coverage.out | tail -n 1

vet:
	gofmt -l . && go vet ./...

docker:
	docker build -t genepath-runner:$(VERSION) .

docker-run:
	docker run --rm -p 8080:8080 -v genepath-data:/data genepath-runner:$(VERSION)

clean:
	rm -rf bin coverage.out

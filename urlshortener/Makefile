.PHONY: build test run migrate clean

build:
	go build -o bin/urlshort ./cmd/urlshort

test:
	go test ./... -race -cover

run: build
	./bin/urlshort serve

migrate: build
	./bin/urlshort migrate

clean:
	rm -rf bin

VERSION=0.2.16

check:
	go test -v .
	go test -race

bench:
	go test -bench . -benchmem ./...

lint:
	golangci-lint run ./...
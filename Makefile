run:
	@go run ./cmd/main.go

test:
	@go test -v ./...

bench:
	@go test ./... -bench . -benchmem
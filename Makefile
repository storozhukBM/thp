golangci := go run github.com/golangci/golangci-lint/cmd/golangci-lint@v1.50.1
gofumpt := go run mvdan.cc/gofumpt@v0.4.0

coverage: test
	go tool cover -html=coverage.out

test: clean format
	go test -race -coverprofile coverage.out ./...

lint: clean
	$(golangci) run .

clean:
	@go clean
	@rm -f profile.out
	@rm -f coverage.out
	@rm -f result.html

format:
	$(gofumpt) -l -w .

help:
	@awk '$$1 ~ /^.*:/ {print substr($$1, 0, length($$1)-1)}' Makefile

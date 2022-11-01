benchstat := go run golang.org/x/perf/cmd/benchstat@v0.0.0-20220920022801-e8d778a60d07
benchart := go run github.com/storozhukBM/benchart@v1.0.0
golangci := go run github.com/golangci/golangci-lint/cmd/golangci-lint@v1.50.1
gofumpt := go run mvdan.cc/gofumpt@v0.4.0

coverage: test
	go tool cover -html=coverage.out

test: clean format
	go test -race -coverprofile coverage.out ./...

bench:
	go test -timeout 3h -count=5 -run=xxx -bench=BenchmarkChanThroughput ./... | tee chan_stat.txt
	$(benchstat) chan_stat.txt
	$(benchstat) -csv chan_stat.txt > chan_stat.csv
	$(benchart) 'ChanThroughput;xAxisType=log' chan_stat.csv chan_stat.html
	open chan_stat.html

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

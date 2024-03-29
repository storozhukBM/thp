benchstat := go run golang.org/x/perf/cmd/benchstat@v0.0.0-20220920022801-e8d778a60d07
benchart := go run github.com/storozhukBM/benchart@v1.0.0
golangci := go run github.com/golangci/golangci-lint/cmd/golangci-lint@v1.52.2
gofumpt := go run mvdan.cc/gofumpt@v0.4.0
gci := go run github.com/daixiang0/gci@v0.10.1

BOLD = \033[1m
CLEAR = \033[0m
CYAN = \033[36m

help: ## Display this help
	@awk '\
		BEGIN {FS = ":.*##"; printf "Usage: make $(CYAN)<target>$(CLEAR)\n"} \
		/^[a-z0-9]+([\/]%)?([\/](%-)?[a-z\-0-9%]+)*:.*? ##/ { printf "  $(CYAN)%-15s$(CLEAR) %s\n", $$1, $$2 } \
		/^##@/ { printf "\n$(BOLD)%s$(CLEAR)\n", substr($$0, 5) }' \
		$(MAKEFILE_LIST)

clean: ## Clean intermediate coverage, profiler and benchmark result files
	@go clean
	@rm -f profile.out
	@rm -f coverage.out
	@rm -f result.html

gci: ## Fix imports order
	$(gci) write .

format: gci ## Run formatting
	$(gofumpt) -l -w .

lint: clean ## Run linters
	$(golangci) run ./...

test: clean format ## Run tests
	go test -race -count 1 ./...

qtest: clean ## Run quick tests
	go test ./...

coverage: ## Measure and show coverage profile
	go test -coverprofile coverage.out ./...
	go tool cover -html=coverage.out

cntprofile: clean ## Get counter CPU profile
	go test -run=xxx -bench=BenchmarkCounterThroughput -cpuprofile profile.out
	go tool pprof -http=:8080 profile.out


chanbench: ## Run channel benchmarks and show benchart
	go test -timeout 3h -count=5 -run=xxx -bench=BenchmarkChanThroughput ./... | tee chan_stat.txt
	$(benchstat) chan_stat.txt
	$(benchstat) -csv chan_stat.txt > chan_stat.csv
	$(benchart) 'ChanThroughput;xAxisType=log' chan_stat.csv chan_stat.html
	open chan_stat.html

cntbench: ## Run counter benchmarks and show benchart
	go test -timeout 3h -count=5 -run=xxx -bench=BenchmarkCounterThroughput ./... | tee cnt_stat.txt
	$(benchstat) cnt_stat.txt
	$(benchstat) -csv cnt_stat.txt > cnt_stat.csv
	$(benchart) 'CounterThroughput;xAxisType=log' cnt_stat.csv cnt_stat.html
	open cnt_stat.html

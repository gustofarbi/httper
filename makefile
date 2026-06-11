mkcert:
	mkcert -install
	mkdir -p echo/certs
	mkcert -cert-file echo/certs/localhost+2.pem -key-file echo/certs/localhost+2-key.pem localhost 127.0.0.1 ::1

echo-server:
	go run ./cmd/echo

test:
	go test -v ./...

COVERAGE_THRESHOLD ?= 60

cover:
	go test -coverpkg=./... -coverprofile=coverage.out ./...
	@total=$$(go tool cover -func=coverage.out | awk '/^total:/ {sub(/%/,"",$$3); print $$3}'); \
	echo "total coverage: $$total% (threshold $(COVERAGE_THRESHOLD)%)"; \
	awk -v t="$$total" -v min=$(COVERAGE_THRESHOLD) 'BEGIN { exit (t+0 < min+0) ? 1 : 0 }'

run-all:
	go build
	for file in "testdata"/*.http; do if [ -f "$$file" ]; then echo "Running $$file"; ./httper $$file; fi; done

fmt:
	go fmt ./...

lint:
	golangci-lint run ./...
	
sec:
	gosec -quiet ./...

vet:
	go vet ./...

qa: sec fmt lint vet cover

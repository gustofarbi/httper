mkcert:
	mkcert -install
	mkdir -p echo/certs
	mkcert -cert-file echo/certs/localhost+2.pem -key-file echo/certs/localhost+2-key.pem localhost 127.0.0.1 ::1

echo-server:
	go run ./cmd/echo

# Regenerates internal/grpcecho/*.pb.go from echo.proto. Needs protoc-gen-go
# and protoc-gen-go-grpc on PATH (go install google.golang.org/protobuf/cmd/protoc-gen-go
# google.golang.org/grpc/cmd/protoc-gen-go-grpc); buf itself runs via go run.
proto:
	cd internal/grpcecho && go run github.com/bufbuild/buf/cmd/buf@v1.50.0 generate

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
	
# internal/echo is the local test fixture server that echoes request data
# back by design; gosec's XSS taint analysis (G705) flags exactly that.
sec:
	gosec -quiet -exclude-dir=internal/echo ./...

vet:
	go vet ./...

qa: sec fmt lint vet cover

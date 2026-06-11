mkcert:
	mkcert -install
	mkdir -p echo/certs
	mkcert -cert-file echo/certs/localhost+2.pem -key-file echo/certs/localhost+2-key.pem localhost 127.0.0.1 ::1

echo-server:
	cd echo && go run main.go

test:
	go test -v ./...

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

qa: sec fmt lint vet test run-all

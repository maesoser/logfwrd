BINARY_NAME = logfwrd

# Build targets
build: vet build-amd64 build-arm64 build-arm build-mips

vet:
	go vet
	go fmt

build-amd64:
	CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go build -v -installsuffix cgo -ldflags '-s -w -extldflags "-static"' -o '$(BINARY_NAME)-linux-amd64' main.go

build-arm64:
	CGO_ENABLED=0 GOARCH=arm64 GOOS=linux go build -v -installsuffix cgo -ldflags '-s -w -extldflags "-static"' -o '$(BINARY_NAME)-linux-arm64' main.go

build-arm:
	CGO_ENABLED=0 GOARCH=arm GOOS=linux go build -v -installsuffix cgo -ldflags '-s -w -extldflags "-static"' -o '$(BINARY_NAME)-linux-arm' main.go

build-mips:
	CGO_ENABLED=0 GOARCH=mips GOOS=linux GOMIPS=softfloat go build -v -installsuffix cgo -ldflags '-s -w -extldflags "-static"' -o '$(BINARY_NAME)-linux-mips' main.go

# Run target
run: build-amd64
	./$(BINARY_NAME)-linux-amd64

# Clean target
clean:
	go clean
	rm $(BINARY_NAME)-linux-mips $(BINARY_NAME)-linux-amd64 $(BINARY_NAME)-linux-arm $(BINARY_NAME)-linux-arm64

# Test targets
test:
	go test ./...

test_coverage:
	go test ./... -coverprofile=coverage.out

# Dependency target
dep:
	go mod download
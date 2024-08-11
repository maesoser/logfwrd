BINARY_NAME = logfwrd

# Build targets
build: vet build-amd64 build-arm64 build-arm build-mips build-mikrotik

vet:
	go vet
	go fmt

build-amd64:
	CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go build -ldflags '-s -w -extldflags "-static"' -o '$(BINARY_NAME)-linux-amd64' logfwrd.go
	upx --lzma '$(BINARY_NAME)-linux-amd64'

build-arm64:
	CGO_ENABLED=0 GOARCH=arm64 GOOS=linux go build -ldflags '-s -w -extldflags "-static"' -o '$(BINARY_NAME)-linux-arm64' logfwrd.go
	upx --lzma '$(BINARY_NAME)-linux-arm64'

build-arm:
	CGO_ENABLED=0 GOARCH=arm GOOS=linux go build -ldflags '-s -w -extldflags "-static"' -o '$(BINARY_NAME)-linux-arm' logfwrd.go
	upx --lzma '$(BINARY_NAME)-linux-arm'

build-mips:
	CGO_ENABLED=0 GOARCH=mips GOOS=linux GOMIPS=softfloat go build -ldflags '-s -w -extldflags "-static"' -o '$(BINARY_NAME)-linux-mips' logfwrd.go
	upx --lzma '$(BINARY_NAME)-linux-mips'

build-mikrotik:
	docker buildx build  --no-cache --platform arm64 --output=type=docker -t $(BINARY_NAME) .
	docker save $(BINARY_NAME) > $(BINARY_NAME).tar

# Run target
run: build-amd64
	./$(BINARY_NAME)-linux-amd64

# Clean target
clean:
	go clean
	rm $(BINARY_NAME)-linux-mips $(BINARY_NAME)-linux-amd64 $(BINARY_NAME)-linux-arm $(BINARY_NAME)-linux-arm64 $(BINARY_NAME).tar

# Test targets
test:
	go test ./...

test_coverage:
	go test ./... -coverprofile=coverage.out

# Dependency target
dep:
	go mod download
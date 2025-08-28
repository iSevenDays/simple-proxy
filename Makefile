BINARY_NAME=simple-proxy
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(shell date '+%Y-%m-%d %H:%M:%S')

LDFLAGS=-ldflags "-X main.Version=${VERSION} -X main.GitCommit=${GIT_COMMIT} -X 'main.BuildTime=${BUILD_TIME}'"

.PHONY: build version clean

build:
	go build ${LDFLAGS} -o ${BINARY_NAME}

version:
	@echo "Version: ${VERSION}"
	@echo "Git Commit: ${GIT_COMMIT}" 
	@echo "Build Time: ${BUILD_TIME}"

clean:
	rm -f ${BINARY_NAME}

# Build with version info
build-with-version: build version
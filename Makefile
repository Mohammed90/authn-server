ORG := keratin
PROJECT := authn-server
NAME := $(ORG)/$(PROJECT)
VERSION := 1.5.0
MAIN := main.go routing.go
GOGOPROTO := $(shell go list -f '{{ .Dir }}' -m github.com/gogo/protobuf)
GOGOGOOGLE := $(shell go list -f '{{ .Dir }}' -m github.com/gogo/googleapis)

# https://github.com/TheThingsIndustries/docker-protobuf
# versions of binaries in image:
# ALPINE_VERSION ?= 3.8
# GO_VERSION ?= 1.11.10
# GRPC_GATEWAY_VERSION ?= 1.9.0
# GRPC_JAVA_VERSION ?= 1.20.0
# GRPC_RUST_VERSION ?= 0.6.1
# GRPC_SWIFT_VERSION ?= 0.9.0
# GRPC_VERSION ?= 1.19.1
# GRPC_WEB_VERSION ?= 1.0.4
# PROTOBUF_C_VERSION ?= 1.3.1
# PROTOC_GEN_DOC_VERSION ?= 1.3.0
# PROTOC_GEN_FIELDMASK_VERSION ?= 0.1.2
# PROTOC_GEN_GO_VERSION ?= 1.3.1
# PROTOC_GEN_GOGO_VERSION ?= 1.2.1
# PROTOC_GEN_GOGOTTN_VERSION ?= 3.0.12
# PROTOC_GEN_LINT_VERSION ?= 0.2.1
# PROTOC_GEN_VALIDATE_VERSION ?= 0.0.14
# RUST_PROTOBUF_VERSION ?= 2.6.0
# RUST_VERSION ?= 1.34.2
# SWIFT_VERSION ?= 5.0.1
# UPX_VERSION ?= 3.95
PROTOC := docker run --user `id -u` --rm --mount type=bind,src=$(PWD)/grpc,dst=$(PWD)/grpc -w $(PWD) thethingsindustries/protoc:3.1.5

.PHONY: clean
clean:
	rm -rf dist

init:
	which -s ego || go get github.com/benbjohnson/ego/cmd/ego
	ego server/views

# The Linux builder is a Docker container because that's the easiest way to get the toolchain for
# CGO on a MacOS host.
.PHONY: linux-builder
linux-builder:
	docker build -f Dockerfile.linux -t $(NAME)-linux-builder .

# The Linux target is built using a special Docker image, because this Makefile assumes the host
# machine is running MacOS.
dist/authn-linux64: init
	make linux-builder
	docker run --rm \
		-v $(PWD):/go/src/github.com/$(NAME) \
		-w /go/src/github.com/$(NAME) \
		$(NAME)-linux-builder \
		sh -c " \
			GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -ldflags '-extldflags -static -X main.VERSION=$(VERSION)' -o '$@' \
		"
	bzip2 -c "$@" > dist/authn-linux64.bz2

# The Darwin target is built using the host machine, which this Makefile assumes is running MacOS.
dist/authn-macos64: init
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 go build -ldflags "-X main.VERSION=$(VERSION)" -o "$@"
	bzip2 -c "$@" > dist/authn-macos64.bz2

# The Windows target is built using a MacOS host machine with `brew install mingw-w64`
dist/authn-windows64.exe: init
	GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc go build -ldflags '-X main.VERSION=$(VERSION)' -o '$@'

# The Docker target wraps the linux/amd64 binary
.PHONY: dist/docker
dist/docker: dist/authn-linux64
	docker build --tag $(NAME):latest .

# Build all distributables
.PHONY: dist
dist: dist/docker dist/authn-macos64 dist/authn-linux64 dist/authn-windows64.exe

# Run the server
.PHONY: server
server: init
	docker-compose up -d redis
	DATABASE_URL=sqlite3://localhost/dev \
		REDIS_URL=redis://127.0.0.1:8701/11 \
		go run -ldflags "-X main.VERSION=$(VERSION)" $(MAIN)

# Run tests
.PHONY: test
test: init
	docker-compose up -d redis mysql postgres
	TEST_REDIS_URL=redis://127.0.0.1:8701/12 \
	  TEST_MYSQL_URL=mysql://root@127.0.0.1:8702/authnservertest \
	  TEST_POSTGRES_URL=postgres://postgres@127.0.0.1/postgres?sslmode=disable \
	  go test -v -race ./...

# Run CI tests
.PHONY: test-ci
test-ci: init
	TEST_REDIS_URL=redis://127.0.0.1/1 \
	  TEST_MYSQL_URL=mysql://root@127.0.0.1/test \
	  TEST_POSTGRES_URL=postgres://postgres@127.0.0.1/postgres?sslmode=disable \
	  go test -v -covermode=count -coverprofile=coverage.out ./...

# Run benchmarks
.PHONY: benchmarks
benchmarks:
	docker-compose up -d redis
	TEST_REDIS_URL=redis://127.0.0.1:8701/12 \
		go test -run=XXX -bench=. \
			github.com/keratin/authn-server/server/meta \
			github.com/keratin/authn-server/server/sessions

# Run migrations
.PHONY: migrate
migrate:
	docker-compose up -d redis
	DATABASE_URL=sqlite3://localhost/dev \
		REDIS_URL=redis://127.0.0.1:8701/11 \
		go run -ldflags "-X main.VERSION=$(VERSION)" $(MAIN) migrate

# Cut a release of the current version.
.PHONY: release
release: test dist
	docker push $(NAME):latest
	docker tag $(NAME):latest $(NAME):$(VERSION)
	docker push $(NAME):$(VERSION)
	git tag v$(VERSION)
	git push --tags
	open https://github.com/$(NAME)/releases/tag/v$(VERSION)
	open dist

.PHONY: generate-grpc
generate-grpc:
	$(PROTOC) -I=./grpc --gogo_out=\
	Mgoogle/api/annotations.proto=github.com/gogo/googleapis/google/api,\
	Mgoogle/protobuf/any.proto=github.com/gogo/protobuf/types,\
	Mgoogle/protobuf/duration.proto=github.com/gogo/protobuf/types,\
	Mgoogle/protobuf/empty.proto=github.com/gogo/protobuf/types,\
	Mgoogle/protobuf/struct.proto=github.com/gogo/protobuf/types,\
	Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types,\
	Mgoogle/protobuf/wrappers.proto=github.com/gogo/protobuf/types,plugins=grpc:./grpc \
	--grpc-gateway_out=\
	Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types,\
	Mgoogle/protobuf/duration.proto=github.com/gogo/protobuf/types,\
	Mgoogle/protobuf/empty.proto=github.com/gogo/protobuf/types,\
	Mgoogle/api/annotations.proto=github.com/gogo/googleapis/google/api,\
	Mgoogle/protobuf/field_mask.proto=github.com/gogo/protobuf/types:./grpc \
	grpc/authn.proto grpc/authn-private.proto
	# Workaround for https://github.com/grpc-ecosystem/grpc-gateway/issues/229
	sed -i.bak "s/empty.Empty/types.Empty/g" grpc/authn.pb.gw.go && rm grpc/authn.pb.gw.go.bak
	sed -i.bak "s/empty.Empty/types.Empty/g" grpc/authn.pb.go && rm grpc/authn.pb.go.bak
	sed -i.bak "s/empty.Empty/types.Empty/g" grpc/authn-private.pb.gw.go && rm grpc/authn-private.pb.gw.go.bak
	sed -i.bak "s/empty.Empty/types.Empty/g" grpc/authn-private.pb.go && rm grpc/authn-private.pb.go.bak
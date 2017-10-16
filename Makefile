# Build everything.
.PHONY: all
all:
	go build $$(go list ./...)

# Test everything.
.PHONY: test
test:
	go test $$(go list ./...)

.PHONY: prereqs
prereqs:
	@# Install dep.
	go get -u github.com/golang/dep/cmd/dep
	@# Install gometalinter and linter dependencies.
	go get -u github.com/alecthomas/gometalinter
	gometalinter -i

.PHONY: deps
deps:
	@# Run dep to install dependencies. Using -vendor-only ensures that this detects stale lockfiles
	@# when run in CI.
	dep ensure -vendor-only

.PHONY: lint-strict
lint-strict:
	@# This limits to the default errorset for gometalinter; you may wish to expand this list.
	gometalinter --disable-all --enable=vet --enable=gotype --enable=gotypex $$(go list -f '{{ .Dir }}' ./...)

.PHONY: lint
lint:
	@# Lint with all but gocyclo.
	gometalinter --disable=gocyclo $$(go list -f '{{ .Dir }}' ./...)

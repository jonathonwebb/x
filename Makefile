TMPDIR := tmp

.PHONY: all
all: deps fmt vet test

.PHONY: check
check: deps-check fmt-check vet cover-check

.PHONY: deps
deps:
	go mod tidy -v

.PHONY: deps-check
deps-check:
	go mod tidy -diff
	go mod verify

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: fmt-check
fmt-check:
	test -z "$(shell gofmt -l .)"

.PHONY: vet
vet:
	go vet ./...

.PHONY: test
test:
	go test ./...

.PHONY: test-check
test-check:
	go test -race -count=1 -v ./...

.PHONY: cover
cover: $(TMPDIR)
	go test -v -coverprofile=$(TMPDIR)/cover.out ./...
	go tool cover -func=$(TMPDIR)/cover.out
	go tool cover -html=$(TMPDIR)/cover.out

.PHONY: cover-check
cover-check: $(TMPDIR)
	go test -race -count=1 -v -coverprofile $(TMPDIR)/cover.out ./...

.PHONY: clean
clean:
	rm -rf $(TMPDIR)

$(TMPDIR):
	mkdir -p $(TMPDIR)

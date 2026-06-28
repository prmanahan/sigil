# Sigil — git credential helper that mints GitHub App installation tokens.

# Build the binary
build:
    go build -o sigil .

# Run tests
test:
    go test ./...

# Static analysis
vet:
    go vet ./...

# Format in place
fmt:
    gofmt -w .

# Format check (non-mutating) — fails if anything is unformatted
fmt-check:
    test -z "$(gofmt -l .)"

# Full verification — what dispatch `verify` names
check: vet fmt-check test

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

# Install a COPY of the pre-commit secret scan into this clone's git hooks dir
# (per-clone, bypassable; re-run after editing .githooks/pre-commit)
hooks:
    install -m 755 .githooks/pre-commit "$(git rev-parse --git-common-dir)/hooks/pre-commit"

# Full secret scan: git history, then the working tree minus sibling worktrees (run by hand)
secret-scan:
    gitleaks git --redact --verbose --no-banner .
    gitleaks dir --redact --verbose --no-banner -c .githooks/gitleaks-tree.toml .

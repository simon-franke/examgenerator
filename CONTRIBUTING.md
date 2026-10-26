# Contributing

Create a feature branch and open a pull request targeting `main`. Direct pushes,
force pushes, and deletion of `main` are blocked. Pull requests must be up to date,
have passing **Go checks** and **Vulnerability scan** checks, and have all review
conversations resolved. These rules also apply to administrators. An independent
approval is optional so the owner can maintain the project alone.

Run these checks before requesting review:

```sh
gofmt -w cmd internal prompts
go mod tidy
go vet ./...
go test -race ./...
go build ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
```

Use a current Go release for vulnerability scanning. CI checks the latest patch
release of the Go version declared in go.mod and scans with the current stable Go.
Tests use local HTTP servers; they do not need real model or Discord credentials.
The PDF compiler tests use a stand-in Docker command. Real PDF generation needs
the Docker image described in the README.

Only squash merges are enabled. GitHub deletes merged feature branches
automatically. Dependency updates are proposed weekly by Dependabot and must pass
the same checks.

Do not commit credentials, course PDFs, generated exams, or traces. Keep local
configuration in `.env`; `.env.example` must contain placeholders only.

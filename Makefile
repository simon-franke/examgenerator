.PHONY: build tidy run-ingest run-generate test

build:
	mkdir -p bin
	go build -o bin/examgen ./cmd/examgen

tidy:
	go mod tidy

test:
	go test ./...

run-ingest: build
	./bin/examgen ingest --slides slides.pdf

run-generate: build
	./bin/examgen generate --exam old-exam.pdf --out new-exam.tex

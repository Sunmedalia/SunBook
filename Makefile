.PHONY: build test check run smoke bench clean

build:
	go build -trimpath -o sunbook ./cmd/sunbook

run:
	go run ./cmd/sunbook .

test:
	go test ./...

check:
	go test -race ./...
	go vet ./...

smoke: build
	python3 -m unittest discover -s scripts -p 'test_*.py'
	python3 scripts/smoke_tui.py ./sunbook

bench:
	go test ./internal/text ./internal/ui ./internal/editor -run '^$$' -bench 'Benchmark(EditLargeDocument|EditMaterializedSnapshot|DiagnosticRendering|TerminalOutput|DiskChecks|MetadataDiskChecks)$$' -benchmem -count=5

clean:
	rm -f sunbook coverage.out

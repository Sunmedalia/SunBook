.PHONY: build test check run smoke clean

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
	python3 scripts/smoke_tui.py ./sunbook

clean:
	rm -f sunbook coverage.out

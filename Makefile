.PHONY: test build vet regression live-smoke compose-up compose-down migrate-up

test:
	go test ./... -count=1

build:
	go build ./...

vet:
	go vet ./...

regression:
	@bash scripts/regression.sh

live-smoke:
	@bash scripts/live-smoke.sh

compose-up:
	docker compose up -d

compose-down:
	docker compose down

migrate-up:
	@echo "Run migrate service: docker compose run --rm migrate"

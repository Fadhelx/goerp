.PHONY: ci test vet fmt-check frontend-install frontend-typecheck frontend-lint frontend-test frontend-build

.PHONY: frontend-e2e

ci: fmt-check vet test frontend-install frontend-typecheck frontend-lint frontend-test frontend-build frontend-e2e

test:
	go test -timeout=20m ./...

vet:
	go vet ./...

fmt-check:
	test -z "$$(gofmt -l .)"

frontend-install:
	pnpm -C frontend install

frontend-typecheck:
	pnpm -C frontend typecheck

frontend-lint:
	pnpm -C frontend lint

frontend-test:
	pnpm -C frontend test

frontend-build:
	pnpm -C frontend build

frontend-e2e:
	pnpm -C frontend test:e2e

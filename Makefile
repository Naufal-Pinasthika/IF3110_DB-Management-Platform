MODULE := github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform

.PHONY: check test test-unit test-web test-integration test-browser test-all browser-install build

check:
	go vet ./...
	go -C tests vet ./...
	npm --prefix tests run test:syntax
	node --check tests/browser/auth_test.mjs
	node --check tests/browser/fixtures_test.mjs

# Fast default: no PostgreSQL, Chromium, or npm install required.
test: test-unit test-web

test-unit:
	mkdir -p tests/artifacts
	go -C tests test -race -count=1 -coverpkg=$(MODULE)/src/internal/auth,$(MODULE)/src/internal/config -coverprofile=artifacts/unit-coverage.out ./unit/...
	npm --prefix tests test

test-web:
	go -C tests test -race -count=1 ./integration/web ./integration/auth -run 'TestPagesRender|TestRouteBoundaries|TestTemplateEscapesPathValues|TestRouterRequiresSessionAndSameOrigin|TestSessionMiddlewarePreservesRepositoryErrors'

test-integration:
	@test -n "$$TEST_DATABASE_URL" || { echo 'Set TEST_DATABASE_URL to a dedicated PostgreSQL database ending in _test'; exit 1; }
	mkdir -p tests/artifacts
	REQUIRE_INTEGRATION_TESTS=1 go -C tests test -race -count=1 -timeout=3m -coverpkg=$(MODULE)/src/app,$(MODULE)/src/api,$(MODULE)/src/internal/auth,$(MODULE)/src/internal/store,$(MODULE)/src/internal/admin,$(MODULE)/src/internal/access -coverprofile=artifacts/integration-coverage.out ./integration/auth ./integration/admin

browser-install:
	npm --prefix tests ci
	npm --prefix tests run browser:install

test-browser:
	@test -n "$$TEST_DATABASE_URL" || { echo 'Set TEST_DATABASE_URL to a dedicated PostgreSQL database ending in _test'; exit 1; }
	REQUIRE_INTEGRATION_TESTS=1 REQUIRE_BROWSER_TESTS=1 go -C tests test -race -count=1 -timeout=5m ./integration/... -run 'TestAuthBrowserIntegration|TestFrontendBrowserFixtures' -v

build:
	mkdir -p tests/artifacts
	go build -o tests/artifacts/server ./src/cmd/server

test-all: check test test-integration test-browser build

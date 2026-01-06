## General

- This project is a Golang monolith with internal parts in `pkg/` directory, main executable in `cmd/server`, JS widget code in `widget/` and Portal frontend code in `web/`. All dependencies get embedded into the final Golang binary.
- Instead of using `go`, `npm` or any other standard tooling, only use targets defined in the `Makefile` with appropriate names (e.g. `init-` for setup, `build-` for building and `test-` for testing)
- Add only the most important comments, prefer adding logs where necessary instead of comments
- If you're not sure how to run something, look for examples in `Makefile` and CI workflow `.github/workflows/ci.yaml`
- If you change any external Go packages, run `make vendors`

## Environment setup

- Use `make init` to initialize everything for development

## Building instructions

- To build widget script for testing, run `make build-widget-script`
- To build portal/web JS code, run `make build-js` followed by `make copy-static-js`
- To build main server executable, run `make build-server` (or `make build-server-ee` if Enterprise Edition changes were made)

## Testing instructions

- To run all Go unit tests, run `make test-unit`
- To run JS widget tests, run `make test-widget-unit`
- To run a single Go integration test, run `make test-docker-light TEST_NAME=<your-test-name>` (prefer running a single test for debugging). Docker is required.
- To run all Go integration tests, run `make test-docker-light`. Docker is required.
- Do not use underscores in Golang test names
- To get unit tests code coverage, run `make test-unit-cover`
- To get integration tests code coverage, after running integration tests, open `coverage_integration/` directory in repository root

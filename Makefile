VERSION ?= dev
LDFLAGS := -ldflags "-X github.com/WTomas/sufleur-cli/internal/cli.Version=$(VERSION)"

.PHONY: build test lint clean release release-dry

build:
	go build $(LDFLAGS) -o dist/sufleur ./cmd/sufleur

test:
	go test ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf dist/ wrappers/pip/dist/

release:
	@if [ -z "$(VERSION)" ] || [ "$(VERSION)" = "dev" ]; then echo "VERSION is required (e.g. make release VERSION=0.1.0)"; exit 1; fi
	@[ "$$(git symbolic-ref --short HEAD)" = "main" ] || (echo "must be on main"; exit 1)
	@git diff --quiet && git diff --cached --quiet || (echo "working tree dirty"; exit 1)
	rm -rf dist/ wrappers/pip/dist/
	cd wrappers/npm && npm version $(VERSION) --no-git-tag-version
	cd wrappers/pip && hatch version $(VERSION)
	git add wrappers/npm/package.json wrappers/pip/pyproject.toml
	git commit -m "release: v$(VERSION)"
	git tag v$(VERSION)
	git push --follow-tags origin main
	goreleaser release --clean
	bash scripts/build-wheels.sh
	twine upload wrappers/pip/dist/*.whl
	cd wrappers/npm && npm publish --access public
	@echo "released v$(VERSION) — git, GitHub Release, PyPI, npm"

release-dry:
	@if [ -z "$(VERSION)" ] || [ "$(VERSION)" = "dev" ]; then echo "VERSION is required (e.g. make release-dry VERSION=0.1.0.dev1)"; exit 1; fi
	rm -rf dist/ wrappers/pip/dist/
	goreleaser release --snapshot --clean
	cd wrappers/npm && npm version $(VERSION) --no-git-tag-version --allow-same-version
	cd wrappers/pip && hatch version $(VERSION)
	bash scripts/build-wheels.sh
	twine upload --repository testpypi wrappers/pip/dist/*.whl
	cd wrappers/npm && npm publish --dry-run --access public
	git checkout -- wrappers/npm/package.json wrappers/pip/pyproject.toml
	@echo "dry-run v$(VERSION) — TestPyPI uploaded, npm publish simulated, version bumps reverted"

VERSION ?= dev
LDFLAGS := -ldflags "-X github.com/sufleur/cli/internal/cli.Version=$(VERSION)"

.PHONY: build test lint clean release release-local

build:
	go build $(LDFLAGS) -o dist/sufleur ./cmd/sufleur

test:
	go test ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf dist/ wrappers/pip/dist/

# Bumps wrapper versions, commits, tags, and pushes. The tag push triggers
# .github/workflows/release.yml, which builds binaries with goreleaser, builds
# the per-platform wheels, and publishes to PyPI (Trusted Publishing) + npm
# (NPM_TOKEN). For a TestPyPI rehearsal, run the release-dry.yml workflow
# manually from the Actions tab.
release:
	@if [ -z "$(VERSION)" ] || [ "$(VERSION)" = "dev" ]; then echo "VERSION is required (e.g. make release VERSION=0.1.0)"; exit 1; fi
	@[ "$$(git symbolic-ref --short HEAD)" = "main" ] || (echo "must be on main"; exit 1)
	@git diff --quiet && git diff --cached --quiet || (echo "working tree dirty"; exit 1)
	cd wrappers/npm && npm version $(VERSION) --no-git-tag-version
	cd wrappers/pip && hatch version $(VERSION)
	git add wrappers/npm/package.json wrappers/pip/src/sufleur_cli/__init__.py
	git commit -m "release: v$(VERSION)"
	git tag v$(VERSION)
	git push --follow-tags origin main
	@echo "tagged v$(VERSION) — pushed to origin. CI release.yml is now building and publishing."

# Fully offline rehearsal — proves snapshot binaries, wheels, the pip install
# path, and the npm install.js postinstall path all work. No GitHub Releases /
# PyPI / npm credentials needed. VERSION must be valid as both a semver string
# (for npm version) and PEP 440 (for hatch version); plain X.Y.Z works for both.
release-local:
	@if [ -z "$(VERSION)" ] || [ "$(VERSION)" = "dev" ]; then echo "VERSION is required (e.g. make release-local VERSION=0.1.0)"; exit 1; fi
	rm -rf dist/ wrappers/pip/dist/
	SUFLEUR_LOCAL_VERSION=$(VERSION) goreleaser release --snapshot --clean
	cd wrappers/npm && npm version $(VERSION) --no-git-tag-version --allow-same-version
	cd wrappers/pip && hatch version $(VERSION)
	bash scripts/build-wheels.sh
	bash scripts/verify-local.sh $(VERSION)
	bash scripts/verify-local-npm.sh $(VERSION)
	git checkout -- wrappers/npm/package.json wrappers/pip/pyproject.toml wrappers/pip/src/sufleur_cli/__init__.py
	@echo "local v$(VERSION) verified — no uploads, working tree clean"

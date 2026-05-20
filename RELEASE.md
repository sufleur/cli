# Release process

Releases are cut by tagging on `main`. The tag push triggers
[`.github/workflows/release.yml`](.github/workflows/release.yml), which builds
binaries, wheels, and publishes to PyPI + npm. No publishing happens from a
maintainer's laptop.

## Cutting a release

```bash
git checkout main
git pull --ff-only
make release VERSION=0.1.0
```

`make release` only does git work: bump wrapper versions
(`wrappers/npm/package.json`, `wrappers/pip/src/sufleur_cli/__init__.py`),
commit, tag `v0.1.0`, push. CI takes over from there.

Watch the run at <https://github.com/sufleur/cli/actions/workflows/release.yml>.

## What CI does on tag push

1. **build-binaries** — goreleaser builds the six target binaries
   (linux/darwin/windows × amd64/arm64), creates the GitHub Release at the
   tag, and uploads archives + `checksums.txt`. Needs `contents: write`.
2. **build-wheels** — `bash scripts/build-wheels.sh` produces six per-platform
   wheels.
3. **publish-pypi** — uploads wheels to PyPI via Trusted Publishing
   (GitHub OIDC). No `PYPI_TOKEN` involved. Runs in the `pypi-release`
   environment, which requires reviewer approval.
4. **publish-npm** — `npm publish --access public --provenance` via Trusted
   Publishing (GitHub OIDC). Runs in the `npm-release` environment, which
   requires reviewer approval. The `--provenance` attestation is signed via
   GitHub OIDC.

## Rehearsing before a real release

### Fully offline (no network)

```bash
make release-local VERSION=0.1.0.rc1
```

Builds snapshot binaries, wheels, and exercises both wrapper install paths
(pip wheel install + npm tarball with `SUFLEUR_BINARY_MIRROR` pointing at a
local HTTP server). No uploads, no credentials needed. Reverts the version
bumps on success.

### TestPyPI via CI (requires GitHub OIDC)

Run the `release-dry` workflow manually from the Actions tab with a VERSION
input (e.g. `0.1.0.rc1`). It uploads to TestPyPI under `pypi-release-test`
and runs `npm publish --dry-run`. No real publish happens.

```
gh workflow run release-dry.yml -f version=0.1.0.rc1
```

Verify the result with:

```bash
pipx install --index-url https://test.pypi.org/simple/ sufleur-cli==0.1.0.rc1
sufleur --help
```

## Secrets in the repo

| Secret         | Used by             | Notes                                          |
| -------------- | ------------------- | ---------------------------------------------- |
| `GITHUB_TOKEN` | every job (default) | Issued by Actions per-run, no setup            |

PyPI, TestPyPI, and npm use Trusted Publishing — no publish tokens in the repo.

## If something goes wrong

CI is fail-forward. PyPI **does not** allow re-uploading the same version, so
if the PyPI publish fails on a real release, fix forward in `v0.1.1` rather
than retrying the tag. Goreleaser and npm both let you retry the same tag,
but if PyPI already accepted the wheels for that version, a retry can leave
state out of sync — easier to bump and re-cut.

To delete a botched tag entirely (only safe if no artifact has been published
to any registry yet):

```bash
git tag -d v0.1.0
git push origin :refs/tags/v0.1.0
gh release delete v0.1.0
```

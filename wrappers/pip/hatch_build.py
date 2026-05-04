from __future__ import annotations

import os
import shutil
import stat
from pathlib import Path
from typing import Any

from hatchling.builders.hooks.plugin.interface import BuildHookInterface


# Maps SUFLEUR_TARGET (the env var driving the build) to:
#   - the GoReleaser dist/ subdirectory the binary lands in
#   - the binary filename inside that subdirectory
#   - the PEP 425 platform tag for the resulting wheel
#
# GoReleaser dist paths follow `dist/<binary>_<goos>_<goarch>_<variant>/<binary>`.
# The `_v1` (amd64) and `_v8.0` (arm64) variants are pinned in .goreleaser.yaml
# via goamd64/goarm64. For local testing, set SUFLEUR_BINARY_PATH to bypass the
# dist/ lookup entirely.
_TARGETS: dict[str, dict[str, str]] = {
    "darwin_amd64": {
        "dist_subdir": "sufleur_darwin_amd64_v1",
        "binary": "sufleur",
        "tag": "macosx_11_0_x86_64",
    },
    "darwin_arm64": {
        "dist_subdir": "sufleur_darwin_arm64_v8.0",
        "binary": "sufleur",
        "tag": "macosx_11_0_arm64",
    },
    "linux_amd64": {
        "dist_subdir": "sufleur_linux_amd64_v1",
        "binary": "sufleur",
        "tag": "manylinux_2_17_x86_64.manylinux2014_x86_64",
    },
    "linux_arm64": {
        "dist_subdir": "sufleur_linux_arm64_v8.0",
        "binary": "sufleur",
        "tag": "manylinux_2_17_aarch64.manylinux2014_aarch64",
    },
    "windows_amd64": {
        "dist_subdir": "sufleur_windows_amd64_v1",
        "binary": "sufleur.exe",
        "tag": "win_amd64",
    },
    "windows_arm64": {
        "dist_subdir": "sufleur_windows_arm64_v8.0",
        "binary": "sufleur.exe",
        "tag": "win_arm64",
    },
}


class SufleurBinaryBuildHook(BuildHookInterface):
    PLUGIN_NAME = "custom"

    def initialize(self, version: str, build_data: dict[str, Any]) -> None:
        target = os.environ.get("SUFLEUR_TARGET")
        if not target:
            raise ValueError(
                "SUFLEUR_TARGET env var is required. "
                f"Set one of: {', '.join(sorted(_TARGETS))}."
            )
        if target not in _TARGETS:
            raise ValueError(
                f"unknown SUFLEUR_TARGET={target!r}. "
                f"Known targets: {', '.join(sorted(_TARGETS))}."
            )
        spec = _TARGETS[target]
        binary_name = spec["binary"]
        platform_tag = spec["tag"]

        override = os.environ.get("SUFLEUR_BINARY_PATH")
        if override:
            source = Path(override)
        else:
            # Walk up from wrappers/pip/ to the repo root, then into dist/.
            repo_root = Path(self.root).resolve().parent.parent
            source = repo_root / "dist" / spec["dist_subdir"] / binary_name

        if not source.is_file():
            raise FileNotFoundError(
                f"sufleur binary not found at {source}. "
                "Build it first (e.g. `make build` or `goreleaser release --snapshot`), "
                "or set SUFLEUR_BINARY_PATH to point at an existing binary."
            )

        staging_dir = Path(self.root) / "_build_bin"
        staging_dir.mkdir(exist_ok=True)
        staged = staging_dir / binary_name
        shutil.copy2(source, staged)
        if not target.startswith("windows_"):
            staged.chmod(staged.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)

        build_data["shared_scripts"] = {f"_build_bin/{binary_name}": binary_name}
        build_data["tag"] = f"py3-none-{platform_tag}"
        build_data["pure_python"] = False

    def finalize(
        self,
        version: str,
        build_data: dict[str, Any],
        artifact_path: str,
    ) -> None:
        # Different SUFLEUR_TARGETs reuse the same checkout in CI; clear the
        # staged binary so the next build starts clean.
        staging_dir = Path(self.root) / "_build_bin"
        if staging_dir.exists():
            shutil.rmtree(staging_dir)

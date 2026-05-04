from __future__ import annotations

import os
import sys
import sysconfig
from fnmatch import fnmatch


class SufleurNotFound(FileNotFoundError):
    """Raised when the sufleur binary cannot be located on disk."""


def find_sufleur_bin() -> str:
    """Return the absolute path to the bundled `sufleur` binary.

    Mirrors astral-sh/uv's `find_uv_bin` search order so the same set of
    install layouts works: ordinary venv, system pip, `pip install --prefix`,
    `pip install --target`, and `pip install --user`.
    """
    binary_name = "sufleur" + (sysconfig.get_config_var("EXE") or "")

    targets: list[str | None] = [
        sysconfig.get_path("scripts"),
        sysconfig.get_path("scripts", vars={"base": sys.base_prefix}),
        _join(_matching_parents(_module_path(), _site_packages_match()), _scripts_subdir()),
        _join(_matching_parents(_module_path(), "sufleur_cli"), "bin"),
        sysconfig.get_path("scripts", scheme=_user_scheme()),
    ]

    seen: list[str] = []
    for target in targets:
        if not target or target in seen:
            continue
        seen.append(target)
        candidate = os.path.join(target, binary_name)
        if os.path.isfile(candidate):
            return candidate

    locations = "\n".join(f" - {t}" for t in seen)
    raise SufleurNotFound(
        f"Could not find the sufleur binary in any of the following locations:\n{locations}\n"
    )


def _site_packages_match() -> str:
    if sys.platform == "win32":
        return "Lib/site-packages/sufleur_cli"
    return "lib/python*/site-packages/sufleur_cli"


def _scripts_subdir() -> str:
    return "Scripts" if sys.platform == "win32" else "bin"


def _module_path() -> str:
    return os.path.dirname(__file__)


def _matching_parents(path: str | None, match: str) -> str | None:
    if not path:
        return None
    parts = path.split(os.sep)
    match_parts = match.split("/")
    if len(parts) < len(match_parts):
        return None
    if not all(
        fnmatch(part, match_part)
        for part, match_part in zip(reversed(parts), reversed(match_parts))
    ):
        return None
    return os.sep.join(parts[: -len(match_parts)])


def _join(path: str | None, *parts: str) -> str | None:
    if not path:
        return None
    return os.path.join(path, *parts)


def _user_scheme() -> str:
    if sys.version_info >= (3, 10):
        return sysconfig.get_preferred_scheme("user")
    if os.name == "nt":
        return "nt_user"
    if sys.platform == "darwin" and getattr(sys, "_framework", ""):
        return "osx_framework_user"
    return "posix_user"

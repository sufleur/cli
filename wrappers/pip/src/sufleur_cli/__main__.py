from __future__ import annotations

import os
import sys

from sufleur_cli import find_sufleur_bin


def main() -> None:
    binary = find_sufleur_bin()
    if sys.platform == "win32":
        import subprocess

        try:
            result = subprocess.run([binary, *sys.argv[1:]])
        except KeyboardInterrupt:
            sys.exit(2)
        sys.exit(result.returncode)
    else:
        os.execvp(binary, [binary, *sys.argv[1:]])


if __name__ == "__main__":
    main()

#!/usr/bin/env python3

import os
from pathlib import Path
import sys


def config_dir() -> Path:
    return Path(os.environ["TRIM_PKGETC"])


def main() -> int:
    command = sys.argv[1] if len(sys.argv) > 1 else ""
    bind_address = os.environ.get("wizard_bind_address", "").strip()
    port = os.environ.get("wizard_port", "").strip()

    if command == "configure":
        if bool(bind_address) != bool(port):
            return 2
        directory = config_dir()
        directory.mkdir(parents=True, exist_ok=True)
        if not bind_address and not port:
            for name in ("bind_address", "service_port"):
                try:
                    (directory / name).unlink()
                except FileNotFoundError:
                    pass
            return 0
        (directory / "bind_address").write_text(f"{bind_address}\n", encoding="utf-8")
        (directory / "service_port").write_text(f"{port}\n", encoding="utf-8")
        return 0

    if command == "restart":
        value = f"{bind_address}:{port}" if bind_address and port else "disabled"
        (config_dir() / "restart-called").write_text(
            f"{value}\n",
            encoding="utf-8",
        )
        return 0

    return 2


if __name__ == "__main__":
    raise SystemExit(main())

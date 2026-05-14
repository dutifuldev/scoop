#!/usr/bin/env python3
"""Backward-compatible entrypoint.

Use `main.py` as the canonical entrypoint.
"""

from main import main

if __name__ == "__main__":
    raise SystemExit(main())

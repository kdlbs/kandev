#!/usr/bin/env python3
"""Write generated Markdown within GitHub Actions' step-summary size limit."""

import argparse
from pathlib import Path


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--diagnostics-label", required=True)
    parser.add_argument("--diagnostics-url", required=True)
    parser.add_argument("--max-bytes", type=int, default=983_040)
    return parser.parse_args()


def bounded_summary(
    source: bytes,
    *,
    diagnostics_label: str,
    diagnostics_url: str,
    max_bytes: int,
) -> bytes:
    source.decode("utf-8")
    if len(source) <= max_bytes:
        return source

    notice = (
        "> [!WARNING]\n"
        "> This step summary was truncated to stay below GitHub Actions' "
        "1 MiB limit. Full diagnostics are available in the "
        f"[{diagnostics_label}]({diagnostics_url}) artifact.\n\n"
    ).encode("utf-8")
    if len(notice) > max_bytes:
        raise ValueError("max bytes is too small for the truncation notice")

    prefix_budget = max_bytes - len(notice)
    # The full source is valid UTF-8, so ignoring errors can only discard a
    # partial code point at the byte boundary.
    prefix = source[:prefix_budget].decode("utf-8", errors="ignore").encode("utf-8")
    return notice + prefix


def main() -> int:
    args = parse_args()
    output = bounded_summary(
        args.input.read_bytes(),
        diagnostics_label=args.diagnostics_label,
        diagnostics_url=args.diagnostics_url,
        max_bytes=args.max_bytes,
    )
    args.output.write_bytes(output)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

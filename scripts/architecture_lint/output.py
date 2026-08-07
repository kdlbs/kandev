"""Human-readable and GitHub Actions diagnostic output."""

import os

from .model import Diagnostic


def annotation_escape(value: str, *, property_value: bool = False) -> str:
    escaped = value.replace("%", "%25").replace("\r", "%0D").replace("\n", "%0A")
    if property_value:
        escaped = escaped.replace(":", "%3A").replace(",", "%2C")
    return escaped


def print_diagnostics(diagnostics: list[Diagnostic]) -> None:
    for item in sorted(diagnostics, key=lambda value: (value.path, value.line, value.rule, value.message)):
        message = f"[{item.rule}] {item.message}"
        if os.environ.get("GITHUB_ACTIONS") == "true":
            print(
                f"::error file={annotation_escape(item.path, property_value=True)},"
                f"line={item.line}::{annotation_escape(message)}"
            )
        else:
            print(f"{item.path}:{item.line}: {message}")

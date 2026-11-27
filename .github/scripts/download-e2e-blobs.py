#!/usr/bin/env python3
"""Download complete native E2E artifacts before publishing reporter inputs."""

import argparse
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import time


def publish_blobs(stage: Path, output: Path) -> int:
    files = {}
    artifacts = sorted(stage.iterdir())
    if not artifacts:
        raise ValueError("empty blob artifact download")
    for artifact in artifacts:
        if (
            artifact.is_symlink()
            or not artifact.is_dir()
            or not re.fullmatch(r"blob-report-(?:containers-)?[1-9][0-9]*", artifact.name)
        ):
            raise ValueError(f"unexpected blob artifact: {artifact.name}")
        entries = sorted(artifact.iterdir())
        if not entries:
            raise ValueError(f"empty blob artifact: {artifact.name}")
        for source in entries:
            if (
                source.is_symlink()
                or not source.is_file()
                or not (source.suffix == ".zip" or source.name.endswith("-metadata.json"))
            ):
                raise ValueError(f"unexpected blob entry: {source.name}")
            if source.name in files:
                raise ValueError(f"duplicate blob basename: {source.name}")
            files[source.name] = source
    if not any(source.suffix == ".zip" for source in files.values()):
        raise ValueError("empty native blob evidence")
    if output.is_symlink() or (output.exists() and (not output.is_dir() or any(output.iterdir()))):
        raise ValueError("blob destination must be empty")
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="e2e-publish-", dir=output.parent) as temporary:
        ready = Path(temporary) / "ready"
        ready.mkdir()
        for name, source in files.items():
            shutil.copyfile(source, ready / name)
        ready.replace(output)
    return len(files)


def download_blobs(repository: str, run_id: int, output: Path) -> int:
    for attempt in range(3):
        try:
            with tempfile.TemporaryDirectory(prefix="e2e-blobs-") as temporary:
                stage = Path(temporary)
                subprocess.run(
                    [
                        "gh", "run", "download", str(run_id),
                        "--repo", repository,
                        "--pattern", "blob-report-*",
                        "--dir", str(stage),
                    ],
                    check=True,
                )
                return publish_blobs(stage, output)
        except subprocess.CalledProcessError:
            if attempt == 2:
                raise
        time.sleep((10, 30)[attempt])


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", required=True)
    parser.add_argument("--run-id", required=True, type=int)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    if args.run_id < 1:
        parser.error("--run-id must be positive")
    count = download_blobs(args.repo, args.run_id, args.output)
    print(f"Downloaded {count} native blob and metadata files.")


if __name__ == "__main__":
    main()

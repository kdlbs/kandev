#!/usr/bin/env python3
"""Native blob transfer preserves binary evidence and rejects incomplete inputs."""

import importlib.util
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest import mock


SCRIPT = Path(__file__).with_name("download-e2e-blobs.py")


class BlobDownloadTests(unittest.TestCase):
    def setUp(self):
        self.assertTrue(SCRIPT.is_file(), "The native blob transfer entry point is missing")
        spec = importlib.util.spec_from_file_location("blob_download", SCRIPT)
        self.module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.module)
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.output = Path(self.tmp.name) / "reports"

    def seed(self, command, name="blob-report-1", filename="report-normal-1-1-0.zip"):
        self.assertEqual(command[:4], ["gh", "run", "download", "42"])
        self.assertEqual(command[command.index("--repo") + 1], "kdlbs/kandev")
        self.assertEqual(command[command.index("--pattern") + 1], "blob-report-*")
        stage = Path(command[command.index("--dir") + 1])
        folder = stage / name
        folder.mkdir(exist_ok=True)
        (folder / filename).write_bytes(b"\x00\xffnative blob\x00")
        return stage

    def download(self):
        return self.module.download_blobs("kdlbs/kandev", 42, self.output)

    def test_success_preserves_all_binary_blobs_and_metadata(self):
        def run(command, **kwargs):
            stage = self.seed(command)
            self.seed(command, "blob-report-containers-1", "report-containers-1-0.zip")
            (stage / "blob-report-1" / "shard-normal-1-metadata.json").write_bytes(b'{"shard":1}\n')
            self.assertTrue(kwargs["check"])

        with mock.patch.object(self.module.subprocess, "run", side_effect=run), mock.patch.object(self.module.time, "sleep") as sleep:
            self.assertEqual(self.download(), 3)
        self.assertEqual(sorted(p.name for p in self.output.iterdir()), ["report-containers-1-0.zip", "report-normal-1-1-0.zip", "shard-normal-1-metadata.json"])
        self.assertEqual((self.output / "report-normal-1-1-0.zip").read_bytes(), b"\x00\xffnative blob\x00")
        self.assertEqual((self.output / "shard-normal-1-metadata.json").read_bytes(), b'{"shard":1}\n')
        sleep.assert_not_called()

    def test_retry_uses_fresh_staging_without_publishing_partial_files(self):
        stages = []

        def run(command, **_kwargs):
            stage = self.seed(command, filename="partial.zip" if not stages else "complete.zip")
            if stages:
                self.assertNotEqual(stage, stages[0])
                self.assertFalse(stages[0].exists())
            stages.append(stage)
            if len(stages) == 1:
                raise subprocess.CalledProcessError(1, command)

        with mock.patch.object(self.module.subprocess, "run", side_effect=run), mock.patch.object(self.module.time, "sleep") as sleep:
            self.assertEqual(self.download(), 1)
        self.assertEqual([p.name for p in self.output.iterdir()], ["complete.zip"])
        self.assertTrue(all(not p.exists() for p in stages))
        sleep.assert_called_once_with(10)

    def test_exhaustion_preserves_three_attempts_and_existing_delays(self):
        stages = []

        def run(command, **_kwargs):
            stages.append(self.seed(command))
            raise subprocess.CalledProcessError(1, command)

        with mock.patch.object(self.module.subprocess, "run", side_effect=run) as runner, mock.patch.object(self.module.time, "sleep") as sleep:
            with self.assertRaises(subprocess.CalledProcessError):
                self.download()
        self.assertEqual(runner.call_count, 3)
        self.assertEqual(sleep.call_args_list, [mock.call(10), mock.call(30)])
        self.assertFalse(self.output.exists())
        self.assertTrue(all(not p.exists() for p in stages))

    def test_duplicate_names_fail_without_overwriting_evidence_or_retrying(self):
        def run(command, **_kwargs):
            self.seed(command, filename="same.zip")
            self.seed(command, "blob-report-2", "same.zip")

        with mock.patch.object(self.module.subprocess, "run", side_effect=run) as runner, mock.patch.object(self.module.time, "sleep") as sleep:
            with self.assertRaisesRegex(ValueError, "duplicate"):
                self.download()
        self.assertFalse(self.output.exists())
        self.assertEqual(runner.call_count, 1)
        sleep.assert_not_called()

    def test_empty_download_fails_without_publishing(self):
        with mock.patch.object(self.module.subprocess, "run"):
            with self.assertRaisesRegex(ValueError, "empty"):
                self.download()
        self.assertFalse(self.output.exists())

    def test_unexpected_files_and_links_fail_before_publication(self):
        for invalid in ("unexpected", "link"):
            with self.subTest(invalid=invalid):
                def run(command, **_kwargs):
                    stage = self.seed(command)
                    path = stage / "blob-report-1" / "extra.zip"
                    if invalid == "link":
                        path.symlink_to(stage / "blob-report-1" / "report-normal-1-1-0.zip")
                    else:
                        (stage / "blob-report-1" / "unexpected.txt").write_text("unexpected")

                with mock.patch.object(self.module.subprocess, "run", side_effect=run):
                    with self.assertRaises(ValueError):
                        self.download()
                self.assertFalse(self.output.exists())

    def test_existing_destination_is_preserved(self):
        self.output.mkdir()
        (self.output / "keep").write_bytes(b"existing evidence")
        with mock.patch.object(self.module.subprocess, "run", side_effect=lambda command, **_kwargs: self.seed(command)):
            with self.assertRaisesRegex(ValueError, "destination"):
                self.download()
        self.assertEqual({p.name: p.read_bytes() for p in self.output.iterdir()}, {"keep": b"existing evidence"})

    def test_copy_failure_cannot_publish_partial_reporter_inputs(self):
        def run(command, **_kwargs):
            self.seed(command)
            self.seed(command, "blob-report-2", "second.zip")

        original = self.module.shutil.copyfile
        copies = []

        def copy(source, destination):
            copies.append(source)
            if len(copies) == 2:
                raise OSError("injected disk failure")
            return original(source, destination)

        with mock.patch.object(self.module.subprocess, "run", side_effect=run), mock.patch.object(self.module.shutil, "copyfile", side_effect=copy):
            with self.assertRaisesRegex(OSError, "injected disk failure"):
                self.download()
        self.assertFalse(self.output.exists())


if __name__ == "__main__":
    unittest.main()

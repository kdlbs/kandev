from __future__ import annotations

import json
import os
import tempfile
import unittest
from pathlib import Path

from nomad_profile_payload import ProfilePayloadError, build_profile_payload, load_original_reference_manifest


class NomadProfilePayloadTests(unittest.TestCase):
    def setUp(self):
        self.source = {
            "schema_version": 1,
            "executor_id": "00000000-0000-0000-0000-000000000002",
            "profile_id": "00000000-0000-0000-0000-000000000003",
            "profile_name": "isolated fixture worker",
            "nomad_token_ref": "$kandev-secret:00000000-0000-0000-0000-000000000004",
            "config": {
                "address": "http://127.0.0.1:26461",
                "namespace": "default",
                "region": "global",
                "datacenter": "dc1",
                "node_id": "00000000-0000-0000-0000-000000000001",
                "platform": "darwin/arm64",
                "ssh_target": "fixture@127.0.0.1",
                "ssh_port": "22422",
                "ssh_host_fingerprint": "SHA256:fixture-host-key",
                "workspace_root": "/private/tmp/fixture/workspace",
                "allocation_root": "/private/tmp/fixture/alloc",
                "cpu_mhz": "48",
                "memory_mb": "1024",
            },
        }

    def load_source(self, source=None):
        temp = tempfile.TemporaryDirectory(prefix="nomad-profile-source-")
        self.addCleanup(temp.cleanup)
        root = Path(temp.name)
        root.chmod(0o700)
        path = root / "original-reference.json"
        fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, "w", encoding="utf-8") as stream:
            json.dump(source if source is not None else self.source, stream)
        return load_original_reference_manifest(path, root)

    def test_original_reference_builds_typed_profile_payload(self):
        original = self.load_source()
        payload = build_profile_payload(original)
        self.assertEqual(payload["name"], self.source["profile_name"])
        self.assertEqual(payload["config"]["nomad_token"], self.source["nomad_token_ref"])
        self.assertEqual(payload["config"]["cpu_mhz"], "48")
        self.assertEqual(set(payload["config"]), set(self.source["config"]) | {"nomad_token"})

    def test_masked_api_projection_is_rejected_before_submission(self):
        projection = {"name": "isolated fixture worker", "config": {"nomad_token": "***"}}
        with self.assertRaises(ProfilePayloadError):
            build_profile_payload(projection)

    def test_masked_token_in_original_manifest_is_rejected(self):
        with self.assertRaises(ProfilePayloadError):
            self.load_source({**self.source, "nomad_token_ref": "***"})

    def test_literal_token_or_extra_token_field_is_rejected(self):
        with self.assertRaises(ProfilePayloadError):
            self.load_source({**self.source, "nomad_token_ref": "plaintext-token"})
        config = dict(self.source["config"])
        config["nomad_token"] = "***"
        with self.assertRaises(ProfilePayloadError):
            self.load_source({**self.source, "config": config})


if __name__ == "__main__":
    unittest.main()

"""TLS plus scoped-ACL fixture mode: config validation, TLS transport, token separation, provisioning, material."""
import json
import unittest
from pathlib import Path

import nomad_native_e2e_bootstrap as bootstrap
import nomad_native_e2e_pool as pool
import nomad_native_e2e_wo06 as wo06
import nomad_native_e2e_wo07 as wo07
from nomad_native_e2e import PreflightError
import nomad_native_e2e_fixture as fixture
from test_nomad_native_e2e_fixture import FixtureBase, TOKEN, World
from test_nomad_native_e2e_wo06 import Wo06Base

SCOPED = "scoped-kandev-token-7a6b5c"
ACCESSOR = "accessor-0001"

SHIPPED_POLICY = Path(__file__).resolve().parent / "fixtures" / "nomad" / "kandev-executor.policy.hcl"
POOL = {"node_pool": "kandev", "placement": "spread"}


class SecurityBase(Wo06Base):
    def secure(self, **security):
        ca = self.parent / "ca.pem"
        ca.write_text("-----BEGIN CERTIFICATE-----\nx\n-----END CERTIFICATE-----\n")
        policy = self.parent / "policy.hcl"
        policy.write_text(SHIPPED_POLICY.read_text())
        self.config["nomad"].update(address="https://127.0.0.1:4646", namespace="kandev")
        self.config["pool"] = dict(POOL)
        self.config["security"] = {"tls": True, "acl_policy": str(policy), "ca_file": str(ca), **security}
        return ca, policy


class SecurityConfigTest(SecurityBase):
    def test_security_is_optional_and_absent_by_default(self):
        self.assertIsNone(self.validate()["security"])

    def test_valid_security_is_normalized_and_sizes_memory_max_to_memory(self):
        ca, policy = self.secure()
        cfg = self.validate()
        self.assertEqual(cfg["security"], {"tls": True, "acl_policy": str(policy), "ca_file": str(ca)})
        self.assertEqual(cfg["pool"]["memory_max_mb"], cfg["memory_mb"])
        self.assertEqual(cfg["pool"]["placement"], "spread")

    def test_rejections(self):
        cases = {
            "not an object": lambda c: c.__setitem__("security", "x"),
            "tls false": lambda c: c["security"].update(tls=False),
            "tls missing": lambda c: c["security"].pop("tls"),
            "policy missing": lambda c: c["security"].pop("acl_policy"),
            "ca missing": lambda c: c["security"].pop("ca_file"),
            "extra key": lambda c: c["security"].update(x=1),
            "relative policy": lambda c: c["security"].update(acl_policy="policy.hcl"),
            "policy absent": lambda c: c["security"].update(acl_policy="/nonexistent/policy.hcl"),
            "ca absent": lambda c: c["security"].update(ca_file="/nonexistent/ca.pem"),
            "http address": lambda c: c["nomad"].update(address="http://127.0.0.1:4646"),
            "non loopback": lambda c: c["nomad"].update(address="https://nomad.example.test:4646"),
            "wrong namespace": lambda c: c["nomad"].update(namespace="default"),
            "no pool": lambda c: c.pop("pool"),
            "wrong pool": lambda c: c["pool"].update(node_pool="default"),
            "binpack": lambda c: c["pool"].update(placement="binpack"),
            "memory max differs": lambda c: c["pool"].update(memory_max_mb=c["memory_mb"] + 1),
        }
        for name, mutate in cases.items():
            with self.subTest(name):
                self.setUp()
                self.secure()
                mutate(self.config)
                with self.assertRaises(PreflightError):
                    self.validate()

    def test_profile_config_carries_the_ca_file_only_in_security_mode(self):
        import nomad_native_e2e_fixture as fixture
        self.config["pool"] = dict(POOL)
        self.assertNotIn("ca_file", fixture.Runner.pool_profile_config(self.validate()))
        ca, _ = self.secure()
        config = fixture.Runner.pool_profile_config(self.validate())
        self.assertEqual(config["ca_file"], str(ca))
        self.assertEqual(config["memory_max_mb"], config["memory_mb"])
        self.assertEqual(config["namespace"], "kandev")
        self.assertEqual(config["placement"], "spread")

    def test_every_pool_scenario_gate_accepts_loopback_https_in_security_mode_only(self):
        self.secure()
        cfg = self.validate()
        for scenario in pool.SCENARIOS:
            pool.check_scenario_config(cfg, scenario)
        wo06.check_scenario_config(cfg, "admission")
        wo07.check_scenario_config(cfg, "cleanup-positive")
        bootstrap.require_loopback_nomad(cfg, "any scenario")
        import nomad_native_e2e_pool_partition as partition
        import nomad_native_e2e_pool_refusals as refusals
        token = self.parent / "identity"
        token.write_text("t")
        token.chmod(0o600)
        self.config["pool"]["host_identity_token_file"] = str(token)
        self.config["ssh_client"] = {"pid": 4242, "config": str(token)}
        refused = self.validate()
        refusals.check_scenario_config(refused)
        partition.check_scenario_config(refused)
        plain = json.loads(json.dumps(cfg))
        plain["security"] = None
        plain["nomad"]["address"] = "https://127.0.0.1:4646"
        for check in (lambda: pool.check_scenario_config(plain, "pool-placement"), lambda: wo06.check_scenario_config(plain, "admission"),
                      lambda: bootstrap.require_loopback_nomad(plain, "x")):
            with self.assertRaises(PreflightError):
                check()


class ShippedPolicyTest(unittest.TestCase):
    def test_policy_matches_the_kandev_executor_contract(self):
        text = SHIPPED_POLICY.read_text()
        for needle in ('namespace "kandev"', '"submit-job"', '"read-job"', 'path "kandev/quarantine/*"', 'node_pool "kandev"',
                       'node {', 'operator {'):
            self.assertIn(needle, text)
        self.assertNotIn("default", text)
        self.assertNotIn('"*"', text)


class AclOpsTest(unittest.TestCase):
    """The default ACL helpers against the Nomad HTTP API shape, through the TLS opener."""

    def test_provision_puts_the_policy_then_creates_a_client_token_bound_to_it(self):
        sent = []

        class Opener:
            def open(self, request, timeout):
                import io
                sent.append((request.get_method(), request.full_url, json.loads(request.data) if request.data else None, request.get_header("X-nomad-token")))
                body = {"AccessorID": "acc", "SecretID": "sec", "Policies": ["kandev-executor"], "Type": "client"} if request.full_url.endswith("/acl/token") else {}
                return io.BytesIO(json.dumps(body).encode())

        original = fixture._nomad_opener
        fixture._nomad_opener = lambda address: Opener()
        self.addCleanup(setattr, fixture, "_nomad_opener", original)
        issued = fixture._default_nomad_acl_provision("https://127.0.0.1:4646", "mgmt", "kandev-executor", "rules")
        self.assertEqual(issued, {"secret": "sec", "accessor": "acc", "policies": ["kandev-executor"], "type": "client"})
        self.assertEqual([item[:2] for item in sent], [("POST", "https://127.0.0.1:4646/v1/acl/policy/kandev-executor"), ("POST", "https://127.0.0.1:4646/v1/acl/token")])
        self.assertEqual(sent[0][2]["Rules"], "rules")
        self.assertEqual(sent[1][2]["Policies"], ["kandev-executor"])
        self.assertEqual(sent[1][2]["Type"], "client")
        self.assertTrue(all(item[3] == "mgmt" for item in sent))

    def test_revoke_deletes_the_token_and_policy_and_tolerates_absence(self):
        import urllib.error
        calls = []

        class Opener:
            def open(self, request, timeout):
                calls.append((request.get_method(), request.full_url))
                if request.full_url.endswith("/acl/policy/kandev-executor"):
                    raise urllib.error.HTTPError(request.full_url, 404, "nf", {}, None)
                import io
                return io.BytesIO(b"")

        original = fixture._nomad_opener
        fixture._nomad_opener = lambda address: Opener()
        self.addCleanup(setattr, fixture, "_nomad_opener", original)
        fixture._default_nomad_acl_revoke("https://127.0.0.1:4646", "mgmt", "acc-1", "kandev-executor")
        self.assertEqual(calls, [("DELETE", "https://127.0.0.1:4646/v1/acl/token/acc-1"), ("DELETE", "https://127.0.0.1:4646/v1/acl/policy/kandev-executor")])

    def test_unsafe_names_are_refused(self):
        for policy in ("management", "a b", "../x", ""):
            with self.assertRaises(PreflightError):
                fixture._default_nomad_acl_provision("https://127.0.0.1:4646", "mgmt", policy, "r")
        with self.assertRaises(PreflightError):
            fixture._default_nomad_acl_revoke("https://127.0.0.1:4646", "mgmt", "../x", "kandev-executor")


class CliTest(unittest.TestCase):
    def test_security_material_subcommand_writes_the_material_and_prints_next_steps(self):
        import contextlib
        import io
        import tempfile
        from nomad_native_e2e import main
        with tempfile.TemporaryDirectory() as tmp:
            out = io.StringIO()
            target = Path(tmp) / "tls"
            with contextlib.redirect_stdout(out):
                self.assertEqual(main(["security-material", "--out", str(target)]), 0)
            self.assertTrue((target / "server.hcl").is_file())
            self.assertIn("nomad acl bootstrap", out.getvalue())
            err = io.StringIO()
            with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(err):
                self.assertEqual(main(["security-material", "--out", str(target)]), 2)
            self.assertIn("security-material failed", err.getvalue())


class MaterialTest(unittest.TestCase):
    def setUp(self):
        import tempfile
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.out = Path(self.temp.name) / "tls"

    def test_commands_request_a_loopback_san_and_short_lived_throwaway_ca(self):
        import nomad_native_e2e_security as security
        calls = []

        def fake(argv, **kwargs):
            calls.append(argv)
            for flag in ("-out", "-keyout"):
                if flag in argv:
                    Path(argv[argv.index(flag) + 1]).write_text("pem")
            from types import SimpleNamespace
            return SimpleNamespace(returncode=0, stdout="", stderr="")

        files = security.generate_material(self.out, run=fake)
        joined = "\n".join(" ".join(call) for call in calls)
        self.assertIn("subjectAltName=DNS:localhost,IP:127.0.0.1", (self.out / "server.ext").read_text())
        self.assertIn("-days 3", joined)
        self.assertTrue(all(call[0] == "openssl" for call in calls))
        self.assertEqual(sorted(files), ["ca_file", "cert_file", "key_file", "server_config"])
        self.assertEqual(self.out.stat().st_mode & 0o777, 0o700)

    def test_server_config_enables_tls_acl_spread_and_no_oversubscription(self):
        import nomad_native_e2e_security as security
        text = security.render_server_config(Path("/fx/tls"))
        for needle in ('ca_file   = "/fx/tls/ca.pem"', "verify_https_client = false", "enabled = true",
                       'scheduler_algorithm             = "spread"', "memory_oversubscription_enabled = false"):
            self.assertIn(needle, text)

    def test_generated_server_certificate_verifies_against_the_throwaway_ca(self):
        import shutil
        import subprocess
        if shutil.which("openssl") is None:
            self.skipTest("openssl is not installed")
        import nomad_native_e2e_security as security
        files = security.generate_material(self.out)
        verify = subprocess.run(["openssl", "verify", "-CAfile", files["ca_file"], files["cert_file"]], capture_output=True, text=True)
        self.assertEqual(verify.returncode, 0, verify.stderr)
        text = subprocess.run(["openssl", "x509", "-in", files["cert_file"], "-noout", "-text"], capture_output=True, text=True).stdout
        self.assertIn("127.0.0.1", text)
        self.assertIn("localhost", text)
        self.assertEqual(Path(files["key_file"]).stat().st_mode & 0o777, 0o600)


class TlsTransportTest(unittest.TestCase):
    NODE = "11111111-1111-4111-8111-111111111111"

    def setUp(self):
        import http.server
        import shutil
        import ssl
        import tempfile
        import threading
        if shutil.which("openssl") is None:
            self.skipTest("openssl is not installed")
        import nomad_native_e2e_security as security
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.files = security.generate_material(Path(self.temp.name) / "tls")
        seen = self.seen = []

        class Handler(http.server.BaseHTTPRequestHandler):
            def _answer(self):
                seen.append((self.command, self.path, self.headers.get("X-Nomad-Token")))
                length = int(self.headers.get("Content-Length") or 0)
                if length:
                    self.rfile.read(length)
                body = b"{}"
                self.send_response(200)
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            do_GET = do_POST = do_DELETE = _answer

            def log_message(self, *args):
                pass

        server = http.server.HTTPServer(("127.0.0.1", 0), Handler)
        context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        context.load_cert_chain(self.files["cert_file"], self.files["key_file"])
        server.socket = context.wrap_socket(server.socket, server_side=True)
        self.address = f"https://127.0.0.1:{server.server_address[1]}"
        threading.Thread(target=server.serve_forever, daemon=True).start()
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        import nomad_native_e2e_fixture as fixture
        self.fixture = fixture
        self.addCleanup(fixture.clear_nomad_tls)

    def test_without_a_registered_ca_the_private_ca_is_not_trusted(self):
        with self.assertRaises(PreflightError):
            self.fixture._default_nomad_get(self.address, "tok", "kandev", "global")("/v1/jobs")

    def test_every_helper_verifies_the_server_against_the_registered_ca(self):
        self.fixture.configure_nomad_tls(self.address, self.files["ca_file"])
        self.assertEqual(self.fixture._default_nomad_get(self.address, "tok", "kandev", "global")("/v1/jobs"), {})
        self.fixture._default_nomad_stop(self.address, "tok", "kandev", "kandev-abc123")
        self.fixture._default_nomad_set_eligibility(self.address, "tok", "global", self.NODE, True)
        self.assertEqual(self.fixture._default_nomad_delete_variable(self.address, "tok", "kandev", "kandev/quarantine/" + self.NODE), "removed")
        self.assertEqual([item[0] for item in self.seen], ["GET", "DELETE", "POST", "DELETE"])
        self.assertTrue(all(item[2] == "tok" for item in self.seen))

    def test_node_metadata_client_uses_the_registered_ca(self):
        import nomad_native_e2e_pool_refusals as refusals
        self.fixture.configure_nomad_tls(self.address, self.files["ca_file"])
        meta = refusals.NodeMeta(self.address, "tok", "global")
        self.assertIs(meta.opener, self.fixture._nomad_opener(self.address))
        meta.apply(self.NODE, None)
        self.assertEqual(self.seen[-1][0], "POST")

    def test_registration_is_per_address(self):
        self.fixture.configure_nomad_tls("https://127.0.0.1:1", self.files["ca_file"])
        with self.assertRaises(PreflightError):
            self.fixture._default_nomad_get(self.address, "tok", "kandev", "global")("/v1/jobs")


class SecurityWorld(World):
    """World with a TLS plus ACL cluster: scheduler, namespace, pool and management token reads, and the scoped token ops."""

    def __init__(self, parent):
        super().__init__(parent)
        self.scheduler = {"SchedulerAlgorithm": "spread", "MemoryOversubscriptionEnabled": False}
        self.token_type = "management"
        self.missing = set()
        self.provisioned, self.revoked = [], []
        self.issue_policies = ["kandev-executor"]
        self.revoke_error = None

    def nomad_factory(self, address, token, namespace, region):
        base = super().nomad_factory(address, token, namespace, region)

        def call(path):
            if path == "/v1/operator/scheduler/configuration":
                return {"SchedulerConfig": dict(self.scheduler)}
            if path in ("/v1/namespace/kandev", "/v1/node/pool/kandev"):
                if path in self.missing:
                    raise fixture.PreflightError(f"Nomad GET {path} failed with status 404")
                return {"Name": "kandev"}
            if path == "/v1/acl/token/self":
                return {"Type": self.token_type}
            return base(path)
        return call

    def acl_provision(self, address, token, policy_name, rules):
        assert token == TOKEN
        self.provisioned.append((address, policy_name, rules))
        return {"secret": SCOPED, "accessor": ACCESSOR, "policies": list(self.issue_policies), "type": "client"}

    def acl_revoke(self, address, token, accessor, policy_name):
        assert token == TOKEN
        if self.revoke_error:
            raise fixture.PreflightError(self.revoke_error)
        self.revoked.append((accessor, policy_name))

    def ops(self):
        import dataclasses
        return dataclasses.replace(super().ops(), nomad_acl_provision=self.acl_provision, nomad_acl_revoke=self.acl_revoke)


class SecurityFlowTest(FixtureBase):
    def setUp(self):
        super().setUp()
        self.world = SecurityWorld(self.parent)
        ca = self.tmp / "ca.pem"
        ca.write_text("-----BEGIN CERTIFICATE-----\nx\n-----END CERTIFICATE-----\n")
        self.policy = self.tmp / "policy.hcl"
        self.policy.write_text(SHIPPED_POLICY.read_text())
        self.config["nomad"].update(address="https://127.0.0.1:4646", namespace="kandev")
        self.config["pool"] = dict(POOL)
        self.config["security"] = {"tls": True, "acl_policy": str(self.policy), "ca_file": str(ca)}
        self.configured = []
        self.original = fixture.configure_nomad_tls
        fixture.configure_nomad_tls = lambda address, ca_file: self.configured.append((address, ca_file))
        self.addCleanup(setattr, fixture, "configure_nomad_tls", self.original)

    def profile(self):
        return json.loads(next(b for b in self.world.bodies if "node_pool" in b))

    def test_tls_is_configured_before_any_nomad_read(self):
        self.run_flow()
        self.assertEqual(self.configured, [("https://127.0.0.1:4646", self.config["security"]["ca_file"])])

    def test_kandev_gets_only_the_scoped_token_and_the_runner_keeps_the_management_token(self):
        self.run_flow()
        self.assertEqual(self.world.secret_value, SCOPED)
        self.assertNotEqual(self.world.secret_value, TOKEN)
        self.assertEqual(self.world.provisioned, [("https://127.0.0.1:4646", "kandev-executor", self.policy.read_text())])
        self.assertTrue(all(TOKEN not in body for body in self.world.bodies))
        self.assertEqual([body for body in self.world.bodies if SCOPED in body], [json.dumps({"name": "nomad-fixture-token", "value": SCOPED})])

    def test_profile_is_https_with_the_ca_namespace_pool_spread_and_fixed_memory(self):
        self.run_flow()
        config = self.profile()["config"]
        self.assertEqual(config["address"], "https://127.0.0.1:4646")
        self.assertEqual(config["ca_file"], self.config["security"]["ca_file"])
        self.assertEqual((config["namespace"], config["node_pool"], config["placement"]), ("kandev", "kandev", "spread"))
        self.assertEqual(config["memory_max_mb"], config["memory_mb"])

    def test_neither_token_appears_in_the_evidence_or_state(self):
        self.world.recover_mutation = "fence"
        self.assertEqual(self.run_flow(), 2)
        written = self.everything_written()
        self.assertNotIn(TOKEN, written)
        self.assertNotIn(SCOPED, written)
        state = json.loads((self.fixture_root() / "fixture.json").read_text())
        self.assertEqual(state["scoped_token"], {"accessor": ACCESSOR, "policy": "kandev-executor"})
        self.assertEqual(state["security"], {"ca_file": self.config["security"]["ca_file"]})

    def test_cluster_preconditions_fail_fast_before_a_token_is_issued(self):
        cases = {
            "binpack": lambda w: w.scheduler.update(SchedulerAlgorithm="binpack"),
            "oversubscription": lambda w: w.scheduler.update(MemoryOversubscriptionEnabled=True),
            "no namespace": lambda w: w.missing.add("/v1/namespace/kandev"),
            "no pool": lambda w: w.missing.add("/v1/node/pool/kandev"),
            "not management": lambda w: setattr(w, "token_type", "client"),
        }
        for name, mutate in cases.items():
            with self.subTest(name):
                self.setUp()
                mutate(self.world)
                self.assertEqual(self.run_flow(), 2)
                self.assertEqual(self.world.provisioned, [])
                self.assertEqual(self.evidence()["failed_step"], "create-fixture-objects")

    def test_a_token_wider_than_the_policy_is_refused(self):
        self.world.issue_policies = ["kandev-executor", "other"]
        self.assertEqual(self.run_flow(), 2)
        self.assertIn("scoped", self.evidence()["error"])

    def test_teardown_revokes_the_scoped_token_once(self):
        self.assertEqual(self.run_flow(), 0, self.err)
        self.assertEqual(self.world.revoked, [(ACCESSOR, "kandev-executor")])
        self.assertTrue(self.evidence()["cleanup"]["scoped_token_revoked"])

    def test_cleanup_fixture_revokes_it_after_a_failed_run_with_the_management_token(self):
        self.world.recover_mutation = "fence"
        self.assertEqual(self.run_flow(), 2)
        self.assertEqual(self.world.revoked, [])
        self.world.fence = "owner:fence-content\n"
        self.assertEqual(fixture.run_cleanup_fixture(str(self.fixture_root()), ops=self.world.ops(), out=self.out.append, err=self.err.append), 0, self.err)
        self.assertEqual(self.world.revoked, [(ACCESSOR, "kandev-executor")])
        self.assertEqual(self.configured[-1][0], "https://127.0.0.1:4646")

    def test_a_failed_revoke_fails_cleanup_and_keeps_the_command(self):
        self.world.revoke_error = "Nomad ACL token revoke failed with status 500"
        self.assertEqual(self.run_flow(), 2)
        self.assertIn("revoke failed", self.evidence()["error"])
        self.assertTrue(any(line.startswith("operator cleanup:") for line in self.out))

    def test_default_runs_never_touch_security(self):
        self.world = SecurityWorld(self.parent)
        self.config.pop("security")
        self.config.pop("pool")
        self.config["nomad"].update(address="http://127.0.0.1:4646", namespace="default")
        self.assertEqual(self.run_flow(), 0, self.err)
        self.assertEqual((self.world.provisioned, self.world.revoked, self.configured), ([], [], []))
        self.assertNotIn("scoped_token_revoked", self.evidence()["cleanup"])


if __name__ == "__main__":
    unittest.main()

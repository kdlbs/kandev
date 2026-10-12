"""Exercise daemon startup shell without starting Docker or proving CNI compatibility."""
import os
from pathlib import Path
import subprocess
import tempfile
import textwrap
import unittest


class DaemonRouteTest(unittest.TestCase):
    def run_startup(self, ipv4="", ipv6="", mtu="1400"):
        template = Path(__file__).with_name("pod-template.yaml").read_text()
        script = textwrap.dedent(template.split("          - |\n", 1)[1].split("        securityContext:", 1)[0])
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            commands = {
                "ip": 'case "$*" in "-4 route show default") printf "%s\\n" "$IPV4";; "-6 route show default") printf "%s\\n" "$IPV6";; *) exit 1;; esac',
                "cat": 'test "$1" = /sys/class/net/net1/mtu || exit 1; printf "%s\\n" "$MTU"',
                "dockerd": 'printf "%s\\n" "$@"',
            }
            for name, body in commands.items():
                command = root / name
                command.write_text("#!/bin/sh\n" + body + "\n")
                command.chmod(0o755)
            return subprocess.run(
                ["sh", "-ceu", script], capture_output=True, text=True, timeout=5,
                env={**os.environ, "PATH": f"{root}:{os.environ['PATH']}",
                     "IPV4": ipv4, "IPV6": ipv6, "MTU": mtu},
            )

    def test_non_eth0_default_routes(self):
        for routes in ({"ipv4": "default via 192.0.2.1 dev net1"},
                       {"ipv6": "default via 2001:db8::1 dev net1"}):
            with self.subTest(routes=routes):
                result = self.run_startup(**routes)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn("--mtu=1400\n", result.stdout)
                self.assertIn("--default-network-opt=bridge=com.docker.network.driver.mtu=1400", result.stdout)

    def test_nested_containers_use_a_relative_cgroup_parent(self):
        result = self.run_startup(ipv4="default via 192.0.2.1 dev net1")

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("--exec-opt\nnative.cgroupdriver=cgroupfs\n", result.stdout)
        self.assertIn("--cgroup-parent=docker\n", result.stdout)

    def test_missing_route_and_invalid_mtu_fail_before_daemon(self):
        for options in ({}, {"ipv4": "default dev net1", "mtu": "invalid"},
                        {"ipv4": "default dev net1", "mtu": "500"}):
            with self.subTest(options=options):
                result = self.run_startup(**options)
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn("--host=", result.stdout)


if __name__ == "__main__":
    unittest.main()

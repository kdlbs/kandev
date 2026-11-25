import { execFileSync } from "node:child_process";
import { randomUUID } from "node:crypto";
import path from "node:path";
import { test, expect } from "../../fixtures/ssh-test-base";
import { execInContainer, readRemoteFile } from "../../helpers/ssh";
import { startBastionAndTarget, stopBastionAndTarget } from "../../helpers/ssh-bastion";
import type { SSHBastionHandles } from "../../helpers/ssh-bastion";
import type {
  SSHEnrollment,
  SSHIdentity,
  SSHObservation,
  SSHRoute,
} from "../../../lib/types/http-ssh-connections";

const authorizedKeys = "/home/kandev/.ssh/authorized_keys";
const scenarios = [
  { topology: "direct", mode: "password" },
  { topology: "bastion", mode: "password" },
  { topology: "bastion", mode: "identity" },
] as const;

// @covers AC-EXECUTORS-SSH-CONNECTIONS-003.5, AC-EXECUTORS-SSH-CONNECTIONS-003.6
for (const { topology, mode } of scenarios) {
  test(`${topology} ${mode} enrollment preserves existing keys and authenticates after restart`, async ({
    backend,
    seedData,
  }, testInfo) => {
    test.setTimeout(180_000);
    const release = await backend.useEnv({ KANDEV_FEATURES_SSH_CONNECTION_MANAGER: "true" });
    let handles: SSHBastionHandles | undefined;
    const request = async <T>(suffix: string, body?: unknown): Promise<T> => {
      const response = await fetch(`${backend.baseUrl}/api/v1/ssh/managed${suffix}`, {
        method: body === undefined ? "GET" : "POST",
        headers: { "Content-Type": "application/json" },
        body: body === undefined ? undefined : JSON.stringify(body),
      });
      expect(response.ok, await response.clone().text()).toBe(true);
      return response.json() as Promise<T>;
    };
    const trust = async (route: SSHRoute) => {
      const root = `/connections/${route.destination.id}`;
      const observed = await request<SSHObservation>(`${root}/observe`, {
        expected_stamp: route.stamp,
      });
      return request<SSHRoute>(`${root}/trust`, {
        expected_stamp: observed.stamp,
        receipt: observed.receipt,
        fingerprint: observed.fingerprint,
      });
    };
    try {
      if (topology === "bastion") {
        handles = startBastionAndTarget(
          testInfo.workerIndex,
          path.join(backend.tmpDir, `managed-enrollment-${mode}`),
        );
      }
      const target = handles?.target ?? seedData.sshTarget;
      const before = readRemoteFile(target, authorizedKeys);
      const bastionBefore = handles && readRemoteFile(handles.bastion, authorizedKeys);
      const password = randomUUID();
      if (mode === "password") {
        // This password belongs only to the disposable container and goes through stdin.
        execFileSync("docker", ["exec", "-i", target.containerName, "chpasswd"], {
          input: `kandev:${password}\n`,
          stdio: ["pipe", "pipe", "pipe"],
        });
        execInContainer(target, [
          "sh",
          "-c",
          "printf 'PasswordAuthentication yes\\n' > /etc/ssh/sshd_config.d/00-enrollment-test.conf; kill -HUP 1",
        ]);
      }
      const key = await request<SSHIdentity>("/identities", {
        request_id: randomUUID(),
        label: `${topology} ${mode} enrolled key`,
        kind: "generate",
      });
      const bootstrap = await request<SSHIdentity>("/identities", {
        request_id: randomUUID(),
        label: "fixture existing key",
        kind: "file",
        reference: target.identityFile,
      });
      let bastion: SSHRoute | undefined;
      if (handles) {
        bastion = await trust(
          await request<SSHRoute>("/connections", {
            request_id: randomUUID(),
            expected_version: 0,
            connection: {
              name: "enrollment bastion",
              host: handles.bastion.host,
              port: handles.bastion.port,
              user: handles.bastion.user,
              identity_id: bootstrap.id,
            },
          }),
        );
        bastion = await request<SSHRoute>(`/connections/${bastion.destination.id}/test`, {
          expected_stamp: bastion.stamp,
        });
      }
      const route = await trust(
        await request<SSHRoute>("/connections", {
          request_id: randomUUID(),
          expected_version: 0,
          connection: {
            name: "enrollment destination",
            host: target.host,
            port: target.port,
            user: target.user,
            identity_id: key.id,
            bastion_id: bastion?.destination.id,
          },
        }),
      );
      const root = `/connections/${route.destination.id}`;
      expect(readRemoteFile(target, authorizedKeys)).toBe(before);
      const body = {
        request_id: randomUUID(),
        expected_stamp: route.stamp,
        key_fingerprint: key.fingerprint,
        mode,
        ...(mode === "password" ? { password } : { bootstrap_identity_id: bootstrap.id }),
      };
      const result = await request<SSHEnrollment>(`${root}/enroll`, body);
      expect(result.state).toBe("verified");
      const after = readRemoteFile(target, authorizedKeys);
      expect(after.startsWith(before)).toBe(true);
      expect(after.slice(before.length).trim()).toBe(key.public_key.trim());
      if (handles) {
        expect(readRemoteFile(handles.bastion, authorizedKeys)).toBe(bastionBefore);
      }
      expect(await request<SSHEnrollment>(`${root}/enroll`, body)).toEqual(result);
      expect(readRemoteFile(target, authorizedKeys)).toBe(after);
      await backend.restart();
      const saved = await request<SSHRoute>(root);
      expect(saved.latest_enrollment?.state).toBe("verified");
      const tested = await request<SSHRoute>(`${root}/test`, { expected_stamp: saved.stamp });
      expect(tested.destination.verified_route).toBe(saved.stamp);
    } finally {
      if (handles) {
        stopBastionAndTarget(handles);
      } else if (mode === "password") {
        execInContainer(seedData.sshTarget, [
          "sh",
          "-c",
          "usermod -p '*' kandev; rm -f /etc/ssh/sshd_config.d/00-enrollment-test.conf; kill -HUP 1",
        ]);
      }
      await release();
    }
  });
}

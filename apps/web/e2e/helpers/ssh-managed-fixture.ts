import { randomUUID } from "node:crypto";
import path from "node:path";
import type { SSHIdentity, SSHObservation, SSHRoute } from "../../lib/types/http-ssh-connections";
import { DatabaseSync } from "./node-sqlite";
import type { SSHServerHandle } from "./ssh";

export async function managedSSHRequest<T>(
  baseUrl: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const response = await fetch(`${baseUrl}/api/v1/ssh/managed${path}`, {
    method: body === undefined ? "GET" : "POST",
    headers: { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!response.ok) {
    throw new Error(`Managed SSH fixture ${path}: ${response.status} ${await response.text()}`);
  }
  return response.json() as Promise<T>;
}

export async function createSSHRoute(
  baseUrl: string,
  name: string,
  target: SSHServerHandle,
  options: { bastionId?: string; identity?: Pick<SSHIdentity, "kind" | "reference"> } = {},
): Promise<SSHRoute> {
  const identity = await managedSSHRequest<SSHIdentity>(baseUrl, "/identities", {
    kind: "file",
    reference: target.identityFile,
    label: name,
    request_id: randomUUID(),
    ...options.identity,
  });
  return managedSSHRequest<SSHRoute>(baseUrl, "/connections", {
    expected_version: 0,
    request_id: randomUUID(),
    connection: {
      name,
      host: target.host,
      port: target.port,
      user: target.user,
      identity_id: identity.id,
      bastion_id: options.bastionId,
    },
  });
}

export async function createTestedSSHRoute(
  baseUrl: string,
  name: string,
  target: SSHServerHandle,
  bastionId?: string,
): Promise<SSHRoute> {
  return trustAndTestSSHRoute(baseUrl, await createSSHRoute(baseUrl, name, target, { bastionId }));
}

export function readSSHRoute(baseUrl: string, id: string) {
  return managedSSHRequest<SSHRoute>(baseUrl, `/connections/${id}`);
}

export function observeSSHRoute(baseUrl: string, route: SSHRoute) {
  return managedSSHRequest<SSHObservation>(
    baseUrl,
    `/connections/${route.destination.id}/observe`,
    { expected_stamp: route.stamp },
  );
}

export async function trustAndTestSSHRoute(baseUrl: string, route: SSHRoute): Promise<SSHRoute> {
  const root = `/connections/${route.destination.id}`;
  const observation = await observeSSHRoute(baseUrl, route);
  const trusted = await managedSSHRequest<SSHRoute>(baseUrl, `${root}/trust`, {
    expected_stamp: observation.stamp,
    receipt: observation.receipt,
    fingerprint: observation.fingerprint,
  });
  return managedSSHRequest<SSHRoute>(baseUrl, `${root}/test`, { expected_stamp: trusted.stamp });
}

export function sshBinding(route: SSHRoute) {
  return { ssh_connection_id: route.destination.id, ssh_connection_stamp: route.stamp };
}

/** A disposable pre-upgrade row, written directly because current APIs reject inline SSH. */
export function insertLegacySSHFixture(
  tmpDir: string,
  name: string,
  config: Record<string, string>,
) {
  const executorId = randomUUID();
  const profileId = randomUUID();
  const db = new DatabaseSync(path.join(tmpDir, "kandev.db"));
  try {
    db.exec("PRAGMA busy_timeout=5000; BEGIN IMMEDIATE");
    db.prepare(
      "INSERT INTO executors (id, name, type, config, created_at, updated_at) VALUES (?, ?, 'ssh', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
    ).run(executorId, name, JSON.stringify(config));
    db.prepare(
      "INSERT INTO executor_profiles (id, executor_id, name, created_at, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
    ).run(profileId, executorId, name);
    db.exec("COMMIT");
  } finally {
    db.close();
  }
  return { executorId, profileId };
}

export function legacySSHFixtureConfig(target: SSHServerHandle) {
  return {
    ssh_host: target.host,
    ssh_port: String(target.port),
    ssh_user: target.user,
    ssh_identity_source: "file",
    ssh_identity_file: target.identityFile,
    ssh_host_fingerprint: target.hostFingerprint,
  };
}

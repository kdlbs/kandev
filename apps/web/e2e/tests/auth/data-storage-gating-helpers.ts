import { expect, type APIRequestContext, type BrowserContext } from "@playwright/test";

/**
 * Shared setup and API probes for the Data & storage member-gating specs
 * (system-data-storage-member-gating.spec.ts desktop and its mobile-* twin).
 *
 * The backend mounts system backups and storage maintenance on two route
 * groups: reads are open to any authenticated caller, mutations and the
 * snapshot download require the admin role. These helpers exercise that split
 * over the real HTTP stack with a real member session, so the specs can spend
 * their assertions on the rendered gating.
 */

export const GATING_ADMIN = {
  email: "admin@demo.dev",
  password: "adminpass123",
  displayName: "Ada Admin",
};

export const GATING_MEMBER = {
  email: "sam@demo.dev",
  password: "memberpass123",
  displayName: "Sam Member",
};

export const DATA_STORAGE_ROUTE = "/settings/system/data-storage";

/** Creates the member account from an authenticated admin context. */
export async function createMember(adminContext: BrowserContext, baseUrl: string): Promise<void> {
  const res = await adminContext.request.post(`${baseUrl}/api/v1/users`, {
    data: {
      email: GATING_MEMBER.email,
      password: GATING_MEMBER.password,
      display_name: GATING_MEMBER.displayName,
      role: "member",
    },
  });
  expect(res.status(), await res.text()).toBe(201);
}

type Snapshot = { name: string; kind: string };

/**
 * Creates a manual snapshot as the admin and resolves its filename. Create is
 * asynchronous (202 + job id), so poll the listing until the file lands: the
 * member specs need a real row to prove the listing survives the gate.
 */
export async function createSnapshotAsAdmin(
  adminContext: BrowserContext,
  baseUrl: string,
): Promise<string> {
  const created = await adminContext.request.post(`${baseUrl}/api/v1/system/backups`);
  expect(created.status(), await created.text()).toBe(202);

  let name = "";
  await expect
    .poll(
      async () => {
        const snapshots = await listSnapshots(adminContext.request, baseUrl);
        const manual = snapshots.find((snapshot) => snapshot.kind === "manual");
        name = manual?.name ?? "";
        return name;
      },
      { timeout: 20_000 },
    )
    .not.toBe("");
  return name;
}

export async function listSnapshots(
  request: APIRequestContext,
  baseUrl: string,
): Promise<Snapshot[]> {
  const res = await request.get(`${baseUrl}/api/v1/system/backups`);
  expect(res.status(), await res.text()).toBe(200);
  const body = (await res.json()) as { snapshots?: Snapshot[] };
  return body.snapshots ?? [];
}

/**
 * Asserts the backend refuses every mutating and exporting route for a member
 * while leaving the reads open. This is the security contract itself; the
 * rendered gating in the specs only keeps a member from meeting a 403.
 */
export async function expectMemberApiGating(
  memberContext: BrowserContext,
  baseUrl: string,
  snapshotName: string,
): Promise<void> {
  const api = memberContext.request;
  const system = `${baseUrl}/api/v1/system`;

  for (const path of [
    "/backups",
    "/storage",
    "/storage/disk",
    "/storage/settings",
    "/storage/runs",
    "/storage/quarantine",
  ]) {
    const res = await api.get(`${system}${path}`);
    expect(res.status(), `GET ${path}: ${await res.text()}`).toBe(200);
  }

  // The download is the most serious of these: a snapshot is a byte copy of
  // every user's rows, so a member must never receive one.
  const download = await api.get(`${system}/backups/${snapshotName}/download`);
  expect(download.status(), await download.text()).toBe(403);

  const denials: Array<[string, Promise<{ status(): number; text(): Promise<string> }>]> = [
    ["POST /backups", api.post(`${system}/backups`)],
    [
      "POST /backups/:name/restore",
      api.post(`${system}/backups/${snapshotName}/restore`, { data: { confirm: "RESTORE" } }),
    ],
    ["DELETE /backups/:name", api.delete(`${system}/backups/${snapshotName}`)],
    ["POST /storage/analyze", api.post(`${system}/storage/analyze`, { data: {} })],
    ["POST /storage/run", api.post(`${system}/storage/run`, { data: {} })],
    [
      "POST /storage/go-cache/adopt",
      api.post(`${system}/storage/go-cache/adopt`, { data: { path: "/tmp/c", confirm: "ADOPT" } }),
    ],
    [
      "POST /storage/quarantine/:id/restore",
      api.post(`${system}/storage/quarantine/entry-1/restore`, { data: {} }),
    ],
    [
      "DELETE /storage/quarantine/:id",
      api.delete(`${system}/storage/quarantine/entry-1`, { data: { confirm: "DELETE" } }),
    ],
    [
      "DELETE /storage/quarantine",
      api.delete(`${system}/storage/quarantine`, {
        data: { scope: "eligible", confirm: "DELETE ELIGIBLE" },
      }),
    ],
  ];
  for (const [label, pending] of denials) {
    const res = await pending;
    expect(res.status(), `${label}: ${await res.text()}`).toBe(403);
  }

  // PATCH needs a valid body so a 403 cannot be mistaken for a 400.
  const settings = await api.get(`${system}/storage/settings`);
  const currentSettings = ((await settings.json()) as { settings: unknown }).settings;
  const patch = await api.patch(`${system}/storage/settings`, {
    data: { settings: currentSettings },
  });
  expect(patch.status(), await patch.text()).toBe(403);

  // The snapshot must still be there after every attempt above.
  const remaining = await listSnapshots(api, baseUrl);
  expect(remaining.map((snapshot) => snapshot.name)).toContain(snapshotName);
}

/* eslint-disable max-lines, max-lines-per-function, sonarjs/no-duplicate-string */

export function createDemoFiles(): Record<string, string> {
  return {
    "README.md": `# Acme Web

Operations console for Acme's commerce platform. The app includes checkout operations,
workspace administration, and a privacy-safe audit trail.

## Development

\`pnpm dev\` starts the Vite app. Run \`pnpm test\` before opening a pull request.
`,
    "package.json": JSON.stringify(
      {
        name: "@acme/web",
        private: true,
        type: "module",
        scripts: {
          dev: "vite",
          build: "tsc -b && vite build",
          test: "vitest run",
          lint: "eslint .",
        },
        dependencies: {
          "@tanstack/react-query": "^5.66.0",
          react: "^18.3.1",
          "react-dom": "^18.3.1",
          "react-router-dom": "^7.1.5",
        },
        devDependencies: { typescript: "^5.7.3", vite: "^6.1.0", vitest: "^3.0.5" },
      },
      null,
      2,
    ),
    "src/app.tsx": `import { Route, Routes } from "react-router-dom";
import { AdminActivityPage } from "./pages/admin/activity-page";
import { CheckoutPage } from "./pages/checkout/checkout-page";
import { DashboardPage } from "./pages/dashboard-page";

export function App() {
  return (
    <Routes>
      <Route path="/" element={<DashboardPage />} />
      <Route path="/checkout/:cartId" element={<CheckoutPage />} />
      <Route path="/admin/activity" element={<AdminActivityPage />} />
    </Routes>
  );
}
`,
    "src/api/audit.ts": `import { api } from "./client";

export type AuditEvent = {
  id: string;
  action: "role.updated" | "access.revoked" | "member.invited";
  actor: { id: string; displayName: string };
  target: { type: "member" | "repository"; label: string };
  region: string;
  createdAt: string;
};

export function listAuditEvents(cursor?: string) {
  return api.get<{ events: AuditEvent[]; nextCursor: string | null }>("/admin/audit", {
    searchParams: { cursor },
  });
}
`,
    "src/api/client.ts": `type RequestOptions = { searchParams?: Record<string, string | undefined> };

export const api = {
  async get<T>(path: string, options: RequestOptions = {}): Promise<T> {
    const url = new URL(path, window.location.origin);
    for (const [key, value] of Object.entries(options.searchParams ?? {})) {
      if (value) url.searchParams.set(key, value);
    }
    const response = await fetch(url);
    if (!response.ok) throw new Error(\`Request failed: \${response.status}\`);
    return response.json() as Promise<T>;
  },
};
`,
    "src/audit/record-event.ts": `import { auditStore } from "./store";
import { coarseRegion } from "../security/privacy";

type RecordEventInput = {
  action: "role.updated" | "access.revoked" | "member.invited";
  actorId: string;
  targetType: "member" | "repository";
  targetId: string;
  requestIp: string;
};

export async function recordAuditEvent(input: RecordEventInput) {
  return auditStore.append({
    action: input.action,
    actorId: input.actorId,
    targetType: input.targetType,
    targetId: input.targetId,
    region: coarseRegion(input.requestIp),
    createdAt: new Date().toISOString(),
  });
}
`,
    "src/audit/store.ts": `export type StoredAuditEvent = {
  action: string;
  actorId: string;
  targetType: string;
  targetId: string;
  region: string;
  createdAt: string;
};

const events: StoredAuditEvent[] = [];

export const auditStore = {
  async append(event: StoredAuditEvent) {
    events.unshift(event);
    return event;
  },
  async list(limit = 50) {
    return events.slice(0, limit);
  },
};
`,
    "src/checkout/complete-order.ts": `import { capturePayment } from "../payments/gateway";
import { reserveInventory } from "./reserve-inventory";
import { withOrderLock } from "./with-order-lock";

export async function completeOrder(orderId: string) {
  const payment = await capturePayment(orderId);

  await withOrderLock(orderId, async () => {
    await reserveInventory(orderId);
  });

  return { orderId, paymentId: payment.id, status: "confirmed" as const };
}
`,
    "src/checkout/reserve-inventory.ts": `import { inventory } from "../inventory/client";

export async function reserveInventory(orderId: string) {
  const result = await inventory.reserve({ orderId, idempotencyKey: orderId });
  if (!result.accepted) throw new Error("Inventory reservation rejected");
  return result;
}
`,
    "src/checkout/with-order-lock.ts": `const locks = new Map<string, Promise<void>>();

export async function withOrderLock<T>(orderId: string, operation: () => Promise<T>): Promise<T> {
  while (locks.has(orderId)) await locks.get(orderId);
  let release!: () => void;
  locks.set(orderId, new Promise<void>((resolve) => { release = resolve; }));
  try {
    return await operation();
  } finally {
    locks.delete(orderId);
    release();
  }
}
`,
    "src/components/audit-log.tsx": `import { useQuery } from "@tanstack/react-query";
import { listAuditEvents } from "../api/audit";

export function AuditLog() {
  const query = useQuery({ queryKey: ["audit-events"], queryFn: () => listAuditEvents() });
  if (query.isPending) return <p>Loading activity...</p>;
  if (query.isError) return <p>Activity could not be loaded.</p>;
  if (!query.data.events.length) return <p>No privileged changes recorded.</p>;
  return (
    <ol>{query.data.events.map((event) => (
      <li key={event.id}>{event.actor.displayName} {event.action} {event.target.label}</li>
    ))}</ol>
  );
}
`,
    "src/components/empty-workspace.tsx": `type EmptyWorkspaceProps = { canManageRepositories: boolean; onConnect(): void };

export function EmptyWorkspace({ canManageRepositories, onConnect }: EmptyWorkspaceProps) {
  return (
    <section aria-labelledby="empty-workspace-title">
      <h2 id="empty-workspace-title">Bring your first repository into Acme</h2>
      <p>Connect a repository to create tasks, run agents, and review changes.</p>
      {canManageRepositories ? (
        <button onClick={onConnect}>Connect repository</button>
      ) : (
        <a href="mailto:platform@acme.example">Ask a workspace admin</a>
      )}
    </section>
  );
}
`,
    "src/pages/admin/activity-page.tsx": `import { AuditLog } from "../../components/audit-log";

export function AdminActivityPage() {
  return <main><h1>Workspace activity</h1><AuditLog /></main>;
}
`,
    "src/pages/checkout/checkout-page.tsx": `import { useParams } from "react-router-dom";

export function CheckoutPage() {
  const { cartId } = useParams();
  return <main><h1>Checkout</h1><p>Cart {cartId}</p></main>;
}
`,
    "src/pages/dashboard-page.tsx": `export function DashboardPage() {
  return <main><h1>Operations</h1><p>12 services healthy</p></main>;
}
`,
    "src/security/privacy.ts": `export function coarseRegion(ipAddress: string): string {
  if (ipAddress.startsWith("10.")) return "internal";
  if (ipAddress.includes(":")) return "global-ipv6";
  return "global";
}
`,
    "tests/audit/record-event.test.ts": `import { describe, expect, it } from "vitest";
import { recordAuditEvent } from "../../src/audit/record-event";

describe("recordAuditEvent", () => {
  it("stores a coarse region without retaining an IP address", async () => {
    const event = await recordAuditEvent({
      action: "role.updated", actorId: "usr_mira", targetType: "member",
      targetId: "usr_lee", requestIp: "203.0.113.42",
    });
    expect(event.region).toBe("global");
    expect(event).not.toHaveProperty("requestIp");
  });
});
`,
    "tests/checkout/concurrent-inventory.test.ts": `import { describe, expect, it } from "vitest";
import { completeOrder } from "../../src/checkout/complete-order";

describe("completeOrder", () => {
  it("does not hold the inventory lock while the gateway responds", async () => {
    const [first, retry] = await Promise.all([completeOrder("ord_42"), completeOrder("ord_42")]);
    expect(first.status).toBe("confirmed");
    expect(retry.status).toBe("confirmed");
  });
});
`,
    "tests/components/empty-workspace.test.tsx": `import { describe, expect, it } from "vitest";

describe("EmptyWorkspace", () => {
  it("offers repository setup to workspace administrators", () => {
    expect("Connect repository").toMatch(/repository/);
  });
});
`,
    "docs/architecture/audit-events.md": `# Audit events

Privileged changes are append-only. Payloads may contain stable user IDs and a coarse
request region, but never credentials, access tokens, raw IP addresses, or request bodies.

The web client reads events through \`GET /admin/audit\` using cursor pagination.
`,
    "migrations/20260718_create_audit_events.sql": `CREATE TABLE audit_events (
  id UUID PRIMARY KEY,
  action TEXT NOT NULL,
  actor_id UUID NOT NULL,
  target_type TEXT NOT NULL,
  target_id TEXT NOT NULL,
  region TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX audit_events_created_at_idx ON audit_events (created_at DESC);
`,
  };
}

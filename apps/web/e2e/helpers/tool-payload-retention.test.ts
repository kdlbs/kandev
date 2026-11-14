import { describe, expect, it, vi } from "vitest";
import type { Page } from "@playwright/test";
import type { ToolPayloadPolicy } from "../../lib/types/tool-payload-retention";
import { withRetentionPolicyRestore } from "./tool-payload-retention";

vi.mock("@playwright/test", async () => ({
  expect: (await import("vitest")).expect,
}));

function policyServer() {
  let policy: ToolPayloadPolicy = {
    enabled: false,
    age: { value: 3, unit: "months" },
    revision: 0,
  };
  let rejectWrites = false;
  const get = async () => ({
    ok: () => true,
    text: async () => JSON.stringify({ policy }),
    json: async () => ({ policy: structuredClone(policy) }),
  });
  const put = async (_url: string, { data }: { data: ToolPayloadPolicy }) => {
    const conflict = rejectWrites || data.revision !== policy.revision;
    if (!conflict) policy = { ...structuredClone(data), revision: policy.revision + 1 };
    return {
      status: () => (conflict ? 409 : 200),
      text: async () => (conflict ? "policy revision conflict" : JSON.stringify({ policy })),
      json: async () => ({ policy: structuredClone(policy) }),
    };
  };
  return {
    page: { request: { get, put } } as unknown as Page,
    changePolicy: async () => {
      await put("policy", {
        data: { enabled: true, age: { value: 2, unit: "weeks" }, revision: 0 },
      });
      await put("policy", {
        data: { enabled: true, age: { value: 4, unit: "weeks" }, revision: 1 },
      });
    },
    failRestoration: () => {
      rejectWrites = true;
    },
    policy: () => structuredClone(policy),
  };
}

describe("retention policy restoration", () => {
  it("restores saved values using the revision after intervening policy updates", async () => {
    const server = policyServer();
    const result = await withRetentionPolicyRestore(server.page, async () => {
      await server.changePolicy();
      return "retained amounts unchanged";
    });

    expect(result).toBe("retained amounts unchanged");
    expect(server.policy()).toEqual({
      enabled: false,
      age: { value: 3, unit: "months" },
      revision: 3,
    });
  });

  it("keeps the original assertion when restoration also fails", async () => {
    const server = policyServer();
    const original = new Error("retained amounts changed");
    const failure = await withRetentionPolicyRestore(server.page, async () => {
      server.failRestoration();
      throw original;
    }).catch((error: unknown) => error);

    expect(failure).toBeInstanceOf(AggregateError);
    const errors = (failure as AggregateError).errors;
    expect(errors).toHaveLength(2);
    expect(errors[0]).toBe(original);
    expect(errors[1].message).toContain("policy revision conflict");
  });

  it("rethrows the original failure after successful restoration", async () => {
    const server = policyServer();
    const original = new Error("native amount assertion");
    await expect(
      withRetentionPolicyRestore(server.page, async () => {
        throw original;
      }),
    ).rejects.toBe(original);
  });
});

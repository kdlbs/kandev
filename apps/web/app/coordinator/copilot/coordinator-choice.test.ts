import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import {
  chooseCoordinator,
  clearLastUsed,
  readLastUsed,
  writeLastUsed,
} from "./coordinator-choice";

const KEY = "kandev.coordinatorCopilot.lastUsed.ws-1";
const list = ["c-1", "c-2", "c-3"].map((id) => ({ id }) as Coordinator);

beforeEach(() => window.localStorage.clear());
afterEach(() => vi.restoreAllMocks());

describe("chooseCoordinator", () => {
  it("uses the last used coordinator while the list holds it", () => {
    window.localStorage.setItem(KEY, "c-2");
    expect(chooseCoordinator("ws-1", list)?.id).toBe("c-2");
  });

  it("falls back to the first and forgets a stale stored value", () => {
    window.localStorage.setItem(KEY, "gone");
    expect(chooseCoordinator("ws-1", list)?.id).toBe("c-1");
    expect(window.localStorage.getItem(KEY)).toBeNull();
  });

  it("never writes last used itself", () => {
    chooseCoordinator("ws-1", list);
    expect(window.localStorage.getItem(KEY)).toBeNull();
  });

  it("is scoped to the workspace", () => {
    window.localStorage.setItem("kandev.coordinatorCopilot.lastUsed.ws-2", "c-3");
    expect(chooseCoordinator("ws-1", list)?.id).toBe("c-1");
  });

  it("returns undefined for an empty list", () => {
    expect(chooseCoordinator("ws-1", [])).toBeUndefined();
  });
});

describe("last used storage failures", () => {
  it("reads an unreadable value as absent", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("denied");
    });
    expect(readLastUsed("ws-1")).toBeNull();
    expect(chooseCoordinator("ws-1", list)?.id).toBe("c-1");
  });

  it("ignores a failed write and a failed clear", () => {
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("full");
    });
    vi.spyOn(Storage.prototype, "removeItem").mockImplementation(() => {
      throw new Error("denied");
    });
    expect(() => writeLastUsed("ws-1", "c-2")).not.toThrow();
    expect(() => clearLastUsed("ws-1")).not.toThrow();
  });

  it("treats an empty value as absent", () => {
    window.localStorage.setItem(KEY, "  ");
    expect(readLastUsed("ws-1")).toBeNull();
  });
});

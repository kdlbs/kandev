import { describe, expect, it, vi } from "vitest";

const { fetchJson } = vi.hoisted(() => ({ fetchJson: vi.fn() }));
vi.mock("../client", () => ({ fetchJson }));

const {
  listTurnChangeHistory,
  getTurnChangeHistoryItem,
  listTurnChangeFiles,
  readTurnChangeContent,
} = await import("./turn-changes-api");

describe("turn changes API", () => {
  it("pages compact session history without requesting patch payloads", async () => {
    await listTurnChangeHistory("session-1", 50, 25, { cache: "no-store" });
    expect(fetchJson).toHaveBeenCalledWith(
      "/api/v1/task-sessions/session-1/turn-changes?offset=50&limit=25",
      { cache: "no-store" },
    );
  });

  it("loads a requested historical turn by exact change-set ID", async () => {
    await getTurnChangeHistoryItem("session-1", "set-older", { cache: "no-store" });
    expect(fetchJson).toHaveBeenCalledWith(
      "/api/v1/task-sessions/session-1/turn-changes/set-older",
      { cache: "no-store" },
    );
  });

  it("scopes file metadata and exact retained variants to server IDs", async () => {
    await listTurnChangeFiles("s", "set", "repo-change", { offset: 100, limit: 100 });
    expect(fetchJson).toHaveBeenCalledWith(
      "/api/v1/task-sessions/s/turn-changes/set/repositories/repo-change/files?offset=100&limit=100",
      undefined,
    );
    await readTurnChangeContent("s", "set", "file", "filtered_patch");
    expect(fetchJson).toHaveBeenLastCalledWith(
      "/api/v1/task-sessions/s/turn-changes/set/files/file/content?variant=filtered_patch",
      undefined,
    );
  });

  it("decodes the JSON base64 representation of retained bytes", async () => {
    const { decodeTurnChangeContent } = await import("./turn-changes-api");
    expect(decodeTurnChangeContent(btoa("-old\n+new\n"))).toBe("-old\n+new\n");
  });
});

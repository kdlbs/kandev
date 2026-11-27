import { afterEach, beforeEach, describe, expect, it } from "vitest";
import {
  disposeScenarios,
  setupTransport,
  startScenario,
  transport,
  file,
  folder,
} from "./file-browser-move-settlement.test-helpers";

const ALPHA = "alpha.txt";
const BETA = "beta.txt";
const MOVED_ALPHA = "dest/alpha.txt";
const MOVED_BETA = "dest/beta.txt";
const REFUSAL = "rename refused";
const TOASTS = "toast-container";
const SUITE = "file move settlement";
const OTHER_KEEP = "other/keep.txt";

beforeEach(setupTransport);
afterEach(disposeScenarios);

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.15 AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.16
describe(SUITE, () => {
  it("retains accepted file after authoritative refresh before sibling failure", async () => {
    const browser = await startScenario();
    await browser.drag();
    expect(
      transport.request.mock.calls
        .filter(([action]) => action === "workspace.file.rename")
        .map(([, args]) => args),
    ).toEqual([
      { session_id: browser.id, old_path: ALPHA, new_path: MOVED_ALPHA },
      { session_id: browser.id, old_path: BETA, new_path: MOVED_BETA },
    ]);
    await browser.settle(0, true);
    expect(browser.accepted).toEqual([MOVED_ALPHA]);
    await browser.notify();
    expect(browser.row(MOVED_ALPHA)).not.toBeNull();
    expect(browser.row(ALPHA)).toBeNull();
    await browser.settle(1, false);
    expect(browser.paths().sort()).toEqual([BETA, "dest", MOVED_ALPHA]);
  });

  it("retains accepted file without a workspace event", async () => {
    const browser = await startScenario();
    await browser.drag();
    await browser.settle(0, true);
    await browser.settle(1, false);
    expect(browser.accepted).toEqual([MOVED_ALPHA]);
    expect(browser.paths().sort()).toEqual([BETA, "dest", MOVED_ALPHA]);
  });

  it.each([
    { accepted: 0, order: [0, 1] },
    { accepted: 0, order: [1, 0] },
    { accepted: 1, order: [0, 1] },
    { accepted: 1, order: [1, 0] },
  ])("settles accepted index $accepted in order $order", async ({ accepted, order }) => {
    const browser = await startScenario();
    await browser.drag();
    expect(browser.paths().sort()).toEqual([ALPHA, BETA, "dest"]);
    for (const index of order) await browser.settle(index, index === accepted);
    expect(browser.accepted).toEqual([accepted === 0 ? MOVED_ALPHA : MOVED_BETA]);
    expect(browser.paths().sort()).toEqual(
      accepted === 0 ? [BETA, "dest", MOVED_ALPHA] : [ALPHA, "dest", MOVED_BETA],
    );
    expect(browser.view.getByTestId(TOASTS).textContent).toContain(REFUSAL);
  });

  it.each([true, false])("retains all-success=$0 control", async (success) => {
    const browser = await startScenario();
    await browser.drag();
    await browser.settle(1, success);
    await browser.settle(0, success);
    expect(browser.accepted).toEqual(success ? [MOVED_BETA, MOVED_ALPHA] : []);
    expect(browser.paths().sort()).toEqual(
      success ? ["dest", MOVED_ALPHA, MOVED_BETA] : [ALPHA, BETA, "dest"],
    );
    expect(browser.view.getByTestId(TOASTS).textContent).toEqual(
      success ? "" : expect.stringContaining(REFUSAL),
    );
  });

  it("normalizes a rejected wire request through the actual operations hook", async () => {
    const browser = await startScenario();
    await browser.drag();
    await browser.settle(1, new Error("connection lost"));
    await browser.settle(0, true);
    expect(browser.accepted).toEqual([MOVED_ALPHA]);
    expect(browser.paths().sort()).toEqual([BETA, "dest", MOVED_ALPHA]);
    expect(browser.view.getByTestId(TOASTS).textContent).toContain("connection lost");
  });

  it("reconciles a write completed before its transport rejection", async () => {
    const browser = await startScenario();
    await browser.drag();
    browser.acceptRemotely(1);
    await browser.settle(1, new Error("reply lost after publication"));
    await browser.settle(0, true);
    expect(browser.paths().sort()).toEqual(["dest", MOVED_ALPHA, MOVED_BETA]);
    expect(browser.accepted).toEqual([MOVED_BETA, MOVED_ALPHA]);
    expect(browser.view.getByTestId(TOASTS).textContent).toContain("reply lost after publication");
  });
});

describe(SUITE, () => {
  it("supersedes pre-acceptance reads while a sibling is pending and final reads fail", async () => {
    const browser = await startScenario(
      folder("", [file(ALPHA), file(BETA), folder("dest"), folder("other", [file(OTHER_KEEP)])]),
    );
    await browser.expand("other");
    browser.replaceServer(
      folder("", [
        file(ALPHA),
        file(BETA),
        folder("dest"),
        folder("other", [file(OTHER_KEEP, 42), file("other/added.txt")]),
      ]),
    );
    browser.holdReads();
    await browser.notify([ALPHA, MOVED_ALPHA, OTHER_KEEP]);
    const oldReads = [...browser.reads];
    expect(oldReads.map(({ path }) => path).sort()).toEqual(["", "dest", "other"]);
    await browser.drag();
    await browser.settle(0, true);
    expect(browser.accepted).toEqual([MOVED_ALPHA]);
    expect(browser.row(MOVED_ALPHA)).not.toBeNull();
    expect(browser.row(ALPHA)).toBeNull();
    await browser.releaseReads(oldReads);
    expect(browser.row(MOVED_ALPHA)).not.toBeNull();
    expect(browser.row(ALPHA)).toBeNull();
    expect(browser.row("other/added.txt")).not.toBeNull();
    expect(
      browser
        .cachedTree()
        ?.children?.find((node) => node.path === "other")
        ?.children?.find((node) => node.path === OTHER_KEEP)?.size,
    ).toBe(42);
    browser.failReads();
    await browser.settle(1, false);
    expect(browser.accepted).toEqual([MOVED_ALPHA]);
    expect(browser.row(MOVED_ALPHA)).not.toBeNull();
    expect(browser.row(ALPHA)).toBeNull();
    expect(browser.row(BETA)).not.toBeNull();
    expect(
      browser
        .cachedTree()
        ?.children?.find((node) => node.path === "dest")
        ?.children?.map((node) => node.path),
    ).toEqual([MOVED_ALPHA]);
  });
});

describe(SUITE, () => {
  it("preserves unrelated authoritative addition removal and metadata", async () => {
    const browser = await startScenario(
      folder("", [file(ALPHA), file(BETA), file("remove.txt"), file("keep.txt"), folder("dest")]),
    );
    await browser.drag();
    await browser.settle(0, true);
    browser.replaceServer(
      folder("", [
        file(BETA),
        file("keep.txt", 42),
        file("added.txt"),
        folder("dest", [file(MOVED_ALPHA)]),
      ]),
    );
    await browser.notify(["", MOVED_ALPHA]);
    expect(browser.row("added.txt")).not.toBeNull();
    expect(browser.row("remove.txt")).toBeNull();
    await browser.settle(1, false);
    expect(browser.paths().sort()).toEqual(["added.txt", BETA, "dest", MOVED_ALPHA, "keep.txt"]);
    expect(browser.cachedTree()?.children?.find((node) => node.path === "keep.txt")?.size).toBe(42);
  });

  it("does not insert captured nodes over newer affected-path data", async () => {
    const browser = await startScenario();
    await browser.drag();
    browser.acceptRemotely(0);
    browser.replaceServer(
      folder("", [file(ALPHA, 50), file(BETA), folder("dest", [file(MOVED_ALPHA, 80)])]),
    );
    await browser.notify();
    // Disk accepted the original rename before the newer authoritative read.
    browser.holdReads();
    await browser.settle(0, true);
    await browser.settle(1, false);
    expect(browser.cachedTree()?.children?.find((node) => node.path === ALPHA)?.size).toBe(50);
    expect(browser.row(ALPHA)).not.toBeNull();
    expect(browser.row(MOVED_ALPHA)).not.toBeNull();
    await browser.releaseReads(browser.reads);
  });

  it("lets a newer subscribed refresh supersede pending settlement reads", async () => {
    const browser = await startScenario();
    await browser.drag();
    browser.holdReads();
    await browser.settle(0, true);
    await browser.settle(1, false);
    const oldReads = [...browser.reads];
    expect(oldReads.map(({ path }) => path).sort()).toEqual(["", "dest"]);
    browser.replaceServer(folder("", [file("newer.txt"), folder("dest", [file(MOVED_ALPHA)])]));
    await browser.notify();
    const freshReads = browser.reads.slice(oldReads.length);
    await browser.releaseReads(freshReads);
    expect(browser.paths().sort()).toEqual(["dest", MOVED_ALPHA, "newer.txt"]);
    await browser.releaseReads(oldReads);
    expect(browser.paths().sort()).toEqual(["dest", MOVED_ALPHA, "newer.txt"]);
  });

  it("preserves confirmed paths when settlement tree reads reject", async () => {
    const browser = await startScenario();
    await browser.drag();
    await browser.settle(0, true);
    browser.failReads();
    await browser.settle(1, false);
    expect(browser.paths().sort()).toEqual([BETA, "dest", MOVED_ALPHA]);
  });
});

describe(SUITE, () => {
  it("retains exact captured collision targets and descendant prefixes", async () => {
    const browser = await startScenario(
      folder("", [
        folder("pack", [file("pack/child.txt")]),
        file(BETA),
        folder("dest", [file("dest/pack")]),
      ]),
    );
    await browser.expand("pack");
    await browser.drag(["pack", BETA]);
    expect(browser.renames.map(({ old_path, new_path }) => ({ old_path, new_path }))).toEqual([
      { old_path: "pack", new_path: "dest/pack (1)" },
      { old_path: BETA, new_path: MOVED_BETA },
    ]);
    await browser.settle(0, true);
    await browser.settle(1, false);
    await browser.expand("dest/pack (1)");
    expect(browser.row("dest/pack (1)/child.txt")).not.toBeNull();
    expect(browser.row("pack")).toBeNull();
    expect(browser.row("dest/pack")).not.toBeNull();
    expect(browser.accepted).toEqual(["dest/pack (1)"]);
  });

  it("retains accepted directory after sibling settles first when final reads reject", async () => {
    const browser = await startScenario(
      folder("", [folder("pack", [file("pack/child.txt")]), file(BETA), folder("dest")]),
    );
    await browser.expand("pack");
    await browser.drag(["pack", BETA]);
    await browser.settle(1, true);
    browser.failReads();
    await browser.settle(0, true);
    expect(browser.accepted).toEqual([MOVED_BETA, "dest/pack"]);
    expect(browser.row("dest/pack")).not.toBeNull();
    await browser.expand("dest/pack");
    expect(browser.row("dest/pack/child.txt")).not.toBeNull();
    expect(browser.row("pack")).toBeNull();
    expect(browser.accepted).toEqual([MOVED_BETA, "dest/pack"]);
  });

  // @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.3
  it("shows the reconciled retained tree before a remount read settles", async () => {
    const browser = await startScenario();
    await browser.drag();
    await browser.settle(0, true);
    await browser.settle(1, false);
    await browser.visit(false);
    browser.holdReads();
    await browser.visit(true);
    expect(browser.reads.length).toBeGreaterThan(0);
    expect(browser.paths().sort()).toEqual([BETA, "dest", MOVED_ALPHA]);
    await browser.releaseReads(browser.reads);
  });

  // @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5
  it("does not publish a retired move into a replacement browser", async () => {
    const browser = await startScenario();
    await browser.drag();
    await browser.visit(false);
    browser.holdReads();
    await browser.visit(true);
    const newReads = browser.reads.length;
    await browser.settle(0, true);
    await browser.settle(1, false);
    expect(browser.accepted).toEqual([MOVED_ALPHA]);
    expect(browser.reads).toHaveLength(newReads);
    expect(browser.paths().sort()).toEqual([ALPHA, BETA, "dest"]);
    await browser.releaseReads(browser.reads);
  });
});

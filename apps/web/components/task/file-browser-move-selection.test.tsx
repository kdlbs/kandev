import { act, fireEvent } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import {
  disposeScenarios,
  setupTransport,
  startScenario,
  transport,
  file,
  folder,
} from "./file-browser-move-settlement.test-helpers";

const OTHER_B = "other/b.txt";
const MOVED_CHILD = "dest/src/a.txt";
const FLAT_CHILD = "dest/a.txt";

beforeEach(setupTransport);
afterEach(disposeScenarios);

function selectionTree(destination = folder("dest")) {
  return folder("", [
    folder("src", [
      file("src/a.txt"),
      folder("src/sub", [
        file("src/sub/keep.txt"),
        folder("src/sub/deep", [file("src/sub/deep/a.txt")]),
      ]),
    ]),
    folder("src-other", [file("src-other/a.txt")]),
    folder("other", [file("other/a.txt"), file(OTHER_B)]),
    file("alpha.txt"),
    file("beta.txt"),
    destination,
  ]);
}

type Scenario = Awaited<ReturnType<typeof startScenario>>;

function expectCommands(browser: Scenario, moves: string[][]) {
  expect(
    transport.request.mock.calls
      .filter(([action]) => action === "workspace.file.rename")
      .map(([, args]) => args),
  ).toEqual(moves.map(([old_path, new_path]) => ({ session_id: browser.id, old_path, new_path })));
}

const COMMAND_CASES = [
  {
    name: "outer ancestor selected last at multiple levels",
    selection: ["src/sub/deep/a.txt", "src/sub", "src"],
    expanded: ["src", "src/sub", "src/sub/deep"],
    moves: [["src", "dest/src"]],
  },
  {
    name: "nested directory without a selected outer ancestor",
    selection: ["src/sub/deep/a.txt", "src/sub"],
    expanded: ["src", "src/sub", "src/sub/deep"],
    moves: [["src/sub", "dest/sub"]],
  },
  {
    name: "mixed independent roots retain selection order",
    selection: ["beta.txt", "src/a.txt", OTHER_B, "src"],
    expanded: ["src", "other"],
    moves: [
      ["beta.txt", "dest/beta.txt"],
      [OTHER_B, "dest/b.txt"],
      ["src", "dest/src"],
    ],
  },
  {
    name: "a sibling prefix is independent",
    selection: ["src-other/a.txt", "src", "src/a.txt"],
    expanded: ["src", "src-other"],
    moves: [
      ["src-other/a.txt", FLAT_CHILD],
      ["src", "dest/src"],
    ],
  },
  {
    name: "folder-only control",
    selection: ["src"],
    expanded: [],
    moves: [["src", "dest/src"]],
  },
  {
    name: "child-only control",
    selection: ["src/a.txt"],
    expanded: ["src"],
    moves: [["src/a.txt", FLAT_CHILD]],
  },
  {
    name: "disjoint control",
    selection: ["alpha.txt", OTHER_B],
    expanded: ["other"],
    moves: [
      ["alpha.txt", "dest/alpha.txt"],
      [OTHER_B, "dest/b.txt"],
    ],
  },
  {
    name: "a covered child cannot reserve a destination basename",
    selection: ["src/a.txt", "other/a.txt", "src"],
    expanded: ["src", "other"],
    moves: [
      ["other/a.txt", FLAT_CHILD],
      ["src", "dest/src"],
    ],
  },
];

// @covers AC-UI-FILE-MOVE-SELECTION-001.1 AC-UI-FILE-MOVE-SELECTION-001.2
describe("directory-covering file move selection", () => {
  it.each([
    { name: "folder-first", selection: ["src", "src/a.txt"] },
    { name: "child-first", selection: ["src/a.txt", "src"] },
  ])("moves one root for $name selection and retains its subtree", async ({ selection }) => {
    const browser = await startScenario(selectionTree());
    await browser.expand("src");
    await browser.drag(selection, 1);
    expectCommands(browser, [["src", "dest/src"]]);
    await browser.settle(0, true);
    await browser.expand("dest/src");
    await browser.expand("dest/src/sub");
    await browser.expand("dest/src/sub/deep");
    expect(browser.row("src")).toBeNull();
    expect(browser.row(FLAT_CHILD)).toBeNull();
    expect(browser.row(MOVED_CHILD)).not.toBeNull();
    expect(browser.row("dest/src/sub/keep.txt")).not.toBeNull();
    expect(browser.row("dest/src/sub/deep/a.txt")).not.toBeNull();
    expect(browser.view.getByTestId("toast-container").textContent).toBe("");
  });

  // @covers AC-UI-FILE-MOVE-SELECTION-001.3 AC-UI-FILE-MOVE-SELECTION-001.4
  it.each(COMMAND_CASES)(
    "constructs literal commands for $name",
    async ({ selection, expanded, moves }) => {
      const browser = await startScenario(selectionTree());
      for (const path of expanded) await browser.expand(path);
      await browser.drag(selection, moves.length);
      expectCommands(browser, moves);
      for (let index = 0; index < moves.length; index++) await browser.settle(index, true);
      expect(browser.accepted).toEqual(moves.map(([, destination]) => destination));
    },
  );

  it("moves a Shift range as one directory root", async () => {
    const browser = await startScenario(selectionTree());
    await browser.expand("src");
    await act(async () => {
      fireEvent.click(browser.row("src")!, { ctrlKey: true });
    });
    await act(async () => {
      fireEvent.click(browser.row("src/a.txt")!, { shiftKey: true });
    });
    expect(browser.row("src")!.dataset.selected).toBe("true");
    expect(browser.row("src/a.txt")!.dataset.selected).toBe("true");
    await act(async () => {
      const dataTransfer = new DataTransfer();
      fireEvent.dragStart(browser.row("src/a.txt")!, { dataTransfer });
      fireEvent.drop(browser.row("dest")!, { dataTransfer });
    });
    expectCommands(browser, [["src", "dest/src"]]);
    await browser.settle(0, true);
  });

  // @covers AC-UI-FILE-MOVE-SELECTION-001.4
  it("keeps a colliding directory's children beneath its chosen basename", async () => {
    const browser = await startScenario(
      selectionTree(
        folder("dest", [
          folder("dest/src", [file("dest/src/existing.txt")]),
          folder("dest/src (1)"),
        ]),
      ),
    );
    await browser.expand("src");
    await browser.drag(["src/a.txt", "src"], 1);
    expectCommands(browser, [["src", "dest/src (2)"]]);
    await browser.settle(0, true);
    await browser.expand("dest/src (2)");
    expect(browser.row("dest/src (2)/a.txt")).not.toBeNull();
    expect(browser.row(FLAT_CHILD)).toBeNull();
    await browser.expand("dest/src");
    expect(browser.row("dest/src/existing.txt")).not.toBeNull();
    expect(browser.row("dest/src (1)")).not.toBeNull();
  });

  // @covers AC-UI-FILE-MOVE-SELECTION-001.5
  it("retains independent failure feedback after a covering directory succeeds", async () => {
    const browser = await startScenario(selectionTree());
    await browser.expand("src");
    await browser.drag(["src/a.txt", "beta.txt", "src"], 2);
    expectCommands(browser, [
      ["beta.txt", "dest/beta.txt"],
      ["src", "dest/src"],
    ]);
    await browser.settle(1, true);
    await browser.settle(0, new Error("independent rename refused"));
    expect(browser.row("beta.txt")).not.toBeNull();
    expect(browser.row("dest/beta.txt")).toBeNull();
    await browser.expand("dest/src");
    expect(browser.row(MOVED_CHILD)).not.toBeNull();
    expect(browser.accepted).toEqual(["dest/src"]);
    expect(browser.view.getByTestId("toast-container").textContent).toContain(
      "independent rename refused",
    );
  });
});

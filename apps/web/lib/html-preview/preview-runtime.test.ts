import { describe, expect, it } from "vitest";
import { createPreviewRuntimeSession, findPreviewSnapshotNode } from "./preview-runtime.worker";
import type { PreviewRuntimeOptions, PreviewSnapshot } from "./preview-runtime-types";

function nodeText(snapshot: PreviewSnapshot, id: string): string | undefined {
  return findPreviewSnapshotNode(snapshot.root, id)?.text;
}

const testOptions: PreviewRuntimeOptions = {
  instructionBudget: 100_000,
  wallClockBudgetMs: 200,
  memoryLimitBytes: 4 * 1024 * 1024,
  maxStackSizeBytes: 512 * 1024,
  maxTimers: 4,
  maxEventQueue: 8,
  maxSnapshotBytes: 128 * 1024,
};

describe("preview runtime", () => {
  it("runs inline scripts in the virtual document and dispatches sanitized events", async () => {
    const session = await createPreviewRuntimeSession(testOptions);
    const snapshot = await session.load(`
      <button id="increment">Increment</button>
      <output id="value">0</output>
      <script>
        const button = document.getElementById("increment");
        const output = document.getElementById("value");
        let count = 0;
        document.body.dataset.runtime = "quickjs";
        button.addEventListener("click", () => {
          count += 1;
          output.textContent = String(count);
        });
      </script>
    `);

    expect(nodeText(snapshot, "value")).toBe("0");
    expect(snapshot.root.attributes["data-runtime"]).toBe("quickjs");

    const button = findPreviewSnapshotNode(snapshot.root, "increment");
    expect(button).toBeDefined();
    const next = await session.dispatch({ type: "click", nodeId: button!.id });

    expect(nodeText(next, "value")).toBe("1");
    await session.dispose();
  });

  it("allows ordinary scripts to use their configured instruction budget", async () => {
    const session = await createPreviewRuntimeSession({
      ...testOptions,
      instructionBudget: 100_000,
    });
    const snapshot = await session.load(`
      <output id="checksum"></output>
      <script>
        let checksum = 0;
        for (let index = 0; index < 25000; index += 1) checksum += index;
        document.getElementById("checksum").textContent = String(checksum);
      </script>
    `);

    expect(nodeText(snapshot, "checksum")).toBe("312487500");
    await session.dispose();
  });

  it("starts a fresh wall-clock budget for each interaction", async () => {
    const session = await createPreviewRuntimeSession({
      ...testOptions,
      wallClockBudgetMs: 200,
    });
    const snapshot = await session.load(`
      <button id="button">Run</button>
      <output id="value">0</output>
      <script>
        document.getElementById("button").addEventListener("click", () => {
          document.getElementById("value").textContent = "1";
        });
      </script>
    `);
    const button = findPreviewSnapshotNode(snapshot.root, "button");
    await new Promise((resolve) => setTimeout(resolve, 220));

    const next = await session.dispatch({ type: "click", nodeId: button!.id });

    expect(nodeText(next, "value")).toBe("1");
    await session.dispose();
  });
});

describe("preview runtime isolation", () => {
  it("does not expose browser authority or allow dynamic network resources", async () => {
    const session = await createPreviewRuntimeSession(testOptions);
    const snapshot = await session.load(`
      <output id="capabilities"></output>
      <img id="remote">
      <img id="embedded">
      <script>
        const capabilities = document.getElementById("capabilities");
        const remote = document.getElementById("remote");
        const embedded = document.getElementById("embedded");
        capabilities.textContent = [
          typeof fetch,
          typeof WebSocket,
          typeof window.open,
          typeof navigator,
          typeof eval,
          typeof Function,
        ].join(",");
        remote.setAttribute("src", "https://example.com/remote.png");
        embedded.setAttribute("src", "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP");
        document.body.appendChild(document.createElement("iframe"));
      </script>
    `);

    expect(nodeText(snapshot, "capabilities")).toBe(
      "undefined,undefined,function,undefined,undefined,undefined",
    );
    const remote = findPreviewSnapshotNode(snapshot.root, "remote");
    const embedded = findPreviewSnapshotNode(snapshot.root, "embedded");
    expect(remote?.attributes.src).toBeUndefined();
    expect(embedded?.attributes.src).toMatch(/^data:/);
    expect(snapshot.root.children.some((child) => child.tagName === "iframe")).toBe(false);
    await session.dispose();
  });

  it("keeps navigation inert and exposes only runtime-owned blob resources", async () => {
    const session = await createPreviewRuntimeSession(testOptions);
    const snapshot = await session.load(`
      <a id="static-link" href="https://example.com">Static</a>
      <form id="form" action="https://example.com"><button id="submit">Submit</button></form>
      <img id="owned">
      <script>
        const form = document.getElementById("form");
        const image = document.getElementById("owned");
        const blob = new Blob(["owned"], { type: "text/plain" });
        const url = URL.createObjectURL(blob);
        image.setAttribute("src", url);
        form.setAttribute("action", "https://example.com/submit");
        location.href = "https://example.com/location";
        history.pushState({}, "", "https://example.com/history");
      </script>
    `);

    const link = findPreviewSnapshotNode(snapshot.root, "static-link");
    const form = findPreviewSnapshotNode(snapshot.root, "form");
    const image = findPreviewSnapshotNode(snapshot.root, "owned");
    expect(link?.attributes.href).toBeUndefined();
    expect(form?.attributes.action).toBeUndefined();
    expect(image?.attributes.src).toMatch(/^blob:preview-runtime-/);
    expect(snapshot.resources).toHaveLength(1);
    expect(snapshot.resources[0].content).toBe("owned");
    await session.dispose();
  });

  it("fails closed when a script exceeds the instruction budget", async () => {
    const session = await createPreviewRuntimeSession({
      ...testOptions,
      instructionBudget: 100,
    });

    await expect(session.load("<script>while (true) {}</script>")).rejects.toMatchObject({
      code: "budget-exceeded",
    });
    await session.dispose();
  });
});

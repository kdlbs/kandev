import { expect, it } from "vitest";
import { WorkerPoolManager } from "@pierre/diffs/worker";
import { PIERRE_THEME } from "./colors";
import { resolveTheme } from "@pierre/diffs";
import { registerPierreTheme, updatePierreTheme } from "./pierre-theme";
function renderOptions(): ReturnType<WorkerPoolManager["getDiffRenderOptions"]> {
  return {
    theme: PIERRE_THEME,
    lineDiffType: "word",
    tokenizeMaxLineLength: 500,
    maxLineDiffLength: 250,
    useTokenTransformer: true,
  };
}
it("registers imported data with the actual Pierre theme resolver", async () => {
  const name = registerPierreTheme({
    reference: "theme:test/night",
    base: "dark",
    digest: "aabbcc",
    status: "ready",
    definition: {
      schema_version: 1,
      base: "dark",
      editor: { background: "#272822" },
      syntax: { keyword: { foreground: "#f92672" } },
    },
  });
  expect(name).toBe("kandev-aabbcc");
  const resolved = await resolveTheme(name!);
  expect(resolved.colors?.["editor.background"]).toBe("#272822");
});

it("serializes worker updates so a slower old switch cannot win", async () => {
  let finish!: () => void;
  const calls: string[] = [];
  const pool = {
    getDiffRenderOptions: renderOptions,
    setRenderOptions: async (options: { theme?: unknown }) => {
      calls.push(String(options.theme));
      if (calls.length === 1)
        await new Promise<void>((resolve) => {
          finish = resolve;
        });
    },
  };
  const theme = (digest: string) => ({
    reference: "theme:test/night" as const,
    base: "dark" as const,
    digest,
    status: "ready" as const,
    definition: { schema_version: 1 as const, base: "dark" as const },
  });
  const first = updatePierreTheme(pool, theme("111111"));
  await Promise.resolve();
  await Promise.resolve();
  const second = updatePierreTheme(pool, theme("222222"));
  finish();
  await Promise.all([first, second]);
  expect(calls).toEqual(["kandev-111111", "kandev-222222"]);
});

it("preserves actual manager options on custom and built-in palette changes", async () => {
  const pool = new WorkerPoolManager(
    {
      poolSize: 0,
      workerFactory: () => {
        throw new Error("No workers expected");
      },
    },
    { langs: [], ...renderOptions() },
  );
  try {
    await pool.initialize();
    const before = pool.getDiffRenderOptions();
    await updatePierreTheme(pool, {
      reference: "theme:test/night",
      base: "dark",
      digest: "options-test",
      status: "ready",
      definition: { schema_version: 1, base: "dark" },
    });
    expect(pool.getDiffRenderOptions()).toEqual({ ...before, theme: "kandev-options-test" });
    await updatePierreTheme(pool, {
      reference: "system",
      base: "light",
      digest: "builtin",
      status: "ready",
    });
    expect(pool.getDiffRenderOptions()).toEqual({ ...before, theme: PIERRE_THEME });
  } finally {
    pool.terminate();
  }
});

it("reads current options when the queued switch executes", async () => {
  let options = renderOptions();
  const pool = {
    getDiffRenderOptions: () => options,
    setRenderOptions: async (next: typeof options) => {
      options = next;
    },
  };
  const pending = updatePierreTheme(pool, {
    reference: "dark",
    base: "dark",
    digest: "builtin",
    status: "ready",
  });
  options = { ...options, tokenizeMaxLineLength: 123 };
  await pending;
  expect(options).toMatchObject({ lineDiffType: "word", tokenizeMaxLineLength: 123 });
});

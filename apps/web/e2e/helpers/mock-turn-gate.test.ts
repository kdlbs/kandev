import { mkdtemp, readFile, rm, stat } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import { mockTurnGate } from "./mock-turn-gate";

const roots: string[] = [];
afterEach(async () => {
  await Promise.all(roots.splice(0).map((root) => rm(root, { recursive: true, force: true })));
});

describe("mock turn release markers", () => {
  it("keeps concurrent turns isolated and serializes paths and output as script strings", async () => {
    const root = await mkdtemp(path.join(os.tmpdir(), 'mock-turn-"quoted"-'));
    roots.push(root);
    const completeMessage = 'completed "quoted" message\nsecond line';
    const first = mockTurnGate(root, completeMessage);
    const second = mockTurnGate(root, "other turn complete");
    const marker = (prompt: string) =>
      JSON.parse(prompt.split("\n")[1].slice("e2e:await_file(".length, -1)) as string;
    const firstMarker = marker(first.prompt);
    const secondMarker = marker(second.prompt);
    expect(path.dirname(firstMarker)).toBe(root);
    expect(firstMarker).not.toBe(secondMarker);
    expect(JSON.parse(first.prompt.split("\n")[2].slice("e2e:message(".length, -1))).toBe(
      completeMessage,
    );
    await expect(stat(firstMarker)).rejects.toMatchObject({ code: "ENOENT" });
    await first.release();
    expect((await stat(firstMarker)).isFile()).toBe(true);
    await expect(stat(secondMarker)).rejects.toMatchObject({ code: "ENOENT" });
    await first.release();
    expect(await readFile(firstMarker, "utf8")).toBe("release");
  });
});

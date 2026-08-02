import { readFile } from "node:fs/promises";
import { inflateRawSync } from "node:zlib";
import { test, expect } from "../../fixtures/test-base";

test.describe("System Logs page", () => {
  test("customizes and downloads the default frontend and backend diagnostic ZIP", async ({
    testPage,
    prCapture,
  }) => {
    test.setTimeout(45_000);
    await testPage.goto("/settings/system/logs");

    await expect(testPage.getByTestId("system-page-title")).toHaveText("Logs");
    await expect(
      testPage.getByText("Create a diagnostic ZIP with frontend and backend logs."),
    ).toBeVisible();
    await expect(testPage.getByText("Review before sharing")).toBeVisible();
    await expect(testPage.getByTestId("system-log-tail-card")).toHaveCount(0);
    await prCapture.screenshot("desktop-combined-diagnostic-logs", {
      caption: "System Logs clearly discloses the combined frontend and backend ZIP.",
      fullPage: true,
    });

    await expect(testPage.getByTestId("download-diagnostic-bundle")).toHaveCount(0);
    await testPage.getByTestId("customize-diagnostic-bundle").click();
    const dialog = testPage.getByTestId("diagnostic-bundle-dialog");
    const downloadPromise = testPage.waitForEvent("download");
    await dialog.getByTestId("create-custom-diagnostic-bundle").click();
    const download = await downloadPromise;
    expect(download.suggestedFilename()).toBe("kandev-diagnostic-logs.zip");
    const archivePath = await download.path();
    expect(archivePath).not.toBeNull();
    const archive = readStoredZip(await readFile(archivePath!));
    expect(archive.has("manifest.json")).toBe(true);
    expect([...archive.keys()].some((name) => name.startsWith("backend/"))).toBe(true);
    expect([...archive.keys()].some((name) => name.startsWith("frontend/"))).toBe(true);
    const manifest = JSON.parse(archive.get("manifest.json")!.toString("utf8")) as {
      requested_sources: string[];
    };
    expect(manifest.requested_sources).toEqual(["backend", "frontend"]);
  });

  test("opens one wider source customizer without enabling ACP from the browser", async ({
    testPage,
    prCapture,
  }) => {
    await testPage.setViewportSize({ width: 1440, height: 900 });
    await testPage.goto("/settings/system/logs");
    await testPage.getByTestId("customize-diagnostic-bundle").click();
    const dialog = testPage.getByTestId("diagnostic-bundle-dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText("Runtime index")).toBeVisible();
    await expect(testPage.getByTestId("download-diagnostic-bundle")).toHaveCount(0);
    await expect(testPage.getByTestId("download-diagnostic-bundle-with-acp")).toHaveCount(0);
    const box = await dialog.boundingBox();
    expect(box?.width ?? 0).toBeGreaterThan(800);
    await prCapture.screenshot("desktop-diagnostic-bundle-customizer", {
      caption: "The wider diagnostic bundle customizer keeps source choices easy to scan.",
    });
  });

  test("keeps privacy notes stacked and the bundle action on the content axis", async ({
    testPage,
  }) => {
    await testPage.setViewportSize({ width: 1440, height: 900 });
    await testPage.goto("/settings/system/logs");

    const noMessages = testPage.getByText(
      "Standard bundles do not include chat transcripts, session messages, or agent messages.",
    );
    const incidentalText = testPage.getByText(
      "Incidental text already written into a log entry is not automatically redacted. Review the ZIP before sharing.",
    );
    const description = testPage.getByText(
      "Kandev asks your connected browser tabs for their bounded three-day console history, combines it with retained backend log files, and downloads one ZIP.",
    );
    const action = testPage.getByTestId("customize-diagnostic-bundle");
    const [noMessagesBox, incidentalTextBox, descriptionBox, actionBox] = await Promise.all([
      noMessages.boundingBox(),
      incidentalText.boundingBox(),
      description.boundingBox(),
      action.boundingBox(),
    ]);

    expect(noMessagesBox).not.toBeNull();
    expect(incidentalTextBox).not.toBeNull();
    expect(descriptionBox).not.toBeNull();
    expect(actionBox).not.toBeNull();
    expect(incidentalTextBox!.y).toBeGreaterThan(noMessagesBox!.y + noMessagesBox!.height);
    expect(Math.abs(actionBox!.x - descriptionBox!.x)).toBeLessThanOrEqual(1);
  });
});

function readStoredZip(buffer: Buffer): Map<string, Buffer> {
  const entries = new Map<string, Buffer>();
  const endOfCentralDirectory = buffer.lastIndexOf(Buffer.from("PK\x05\x06", "binary"));
  if (endOfCentralDirectory < 0) throw new Error("diagnostic ZIP has no central directory");
  const count = buffer.readUInt16LE(endOfCentralDirectory + 10);
  let offset = buffer.readUInt32LE(endOfCentralDirectory + 16);
  for (let index = 0; index < count; index += 1) {
    if (buffer.readUInt32LE(offset) !== 0x02014b50) {
      throw new Error("diagnostic ZIP central directory is truncated");
    }
    const flags = buffer.readUInt16LE(offset + 8);
    const method = buffer.readUInt16LE(offset + 10);
    const compressedSize = buffer.readUInt32LE(offset + 20);
    const nameLength = buffer.readUInt16LE(offset + 28);
    const extraLength = buffer.readUInt16LE(offset + 30);
    const commentLength = buffer.readUInt16LE(offset + 32);
    const localOffset = buffer.readUInt32LE(offset + 42);
    const nameStart = offset + 46;
    const name = buffer.subarray(nameStart, nameStart + nameLength).toString("utf8");
    if (flags & 0x1) throw new Error("diagnostic ZIP entry is encrypted");
    if (buffer.readUInt32LE(localOffset) !== 0x04034b50) {
      throw new Error("diagnostic ZIP local entry is truncated");
    }
    const localNameLength = buffer.readUInt16LE(localOffset + 26);
    const localExtraLength = buffer.readUInt16LE(localOffset + 28);
    const dataStart = localOffset + 30 + localNameLength + localExtraLength;
    const dataEnd = dataStart + compressedSize;
    if (dataEnd > buffer.length) throw new Error("diagnostic ZIP entry is truncated");
    const compressed = buffer.subarray(dataStart, dataEnd);
    let data: Buffer | null = null;
    if (method === 0) {
      data = compressed;
    } else if (method === 8) {
      data = inflateRawSync(compressed);
    }
    if (!data) throw new Error(`diagnostic ZIP uses unsupported method=${method}`);
    entries.set(name, data);
    offset += 46 + nameLength + extraLength + commentLength;
  }
  return entries;
}

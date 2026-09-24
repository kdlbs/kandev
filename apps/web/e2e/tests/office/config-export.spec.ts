import { readFileSync } from "node:fs";
import { inflateRawSync } from "node:zlib";
import { expect, test } from "../../fixtures/office-fixture";

function readZipEntries(path: string): Map<string, Buffer> {
  const archive = readFileSync(path);
  const entries = new Map<string, Buffer>();
  const endOfCentralDirectory = 0x06054b50;
  const centralDirectoryEntry = 0x02014b50;
  const localFileEntry = 0x04034b50;
  let end = archive.length - 22;
  while (end >= 0 && archive.readUInt32LE(end) !== endOfCentralDirectory) end -= 1;
  if (end < 0) throw new Error("ZIP end-of-central-directory record not found");
  const count = archive.readUInt16LE(end + 10);
  let offset = archive.readUInt32LE(end + 16);
  for (let index = 0; index < count; index += 1) {
    if (archive.readUInt32LE(offset) !== centralDirectoryEntry)
      throw new Error("invalid ZIP entry");
    const method = archive.readUInt16LE(offset + 10);
    const compressedSize = archive.readUInt32LE(offset + 20);
    const nameLength = archive.readUInt16LE(offset + 28);
    const extraLength = archive.readUInt16LE(offset + 30);
    const commentLength = archive.readUInt16LE(offset + 32);
    const localOffset = archive.readUInt32LE(offset + 42);
    const name = archive.subarray(offset + 46, offset + 46 + nameLength).toString("utf8");
    if (archive.readUInt32LE(localOffset) !== localFileEntry)
      throw new Error("invalid local ZIP entry");
    const localNameLength = archive.readUInt16LE(localOffset + 26);
    const localExtraLength = archive.readUInt16LE(localOffset + 28);
    const dataStart = localOffset + 30 + localNameLength + localExtraLength;
    const compressed = archive.subarray(dataStart, dataStart + compressedSize);
    entries.set(name, method === 8 ? inflateRawSync(compressed) : compressed);
    offset += 46 + nameLength + extraLength + commentLength;
  }
  return entries;
}

test("configuration export downloads the selected server manifest", async ({
  testPage,
  officeSeed: _,
}) => {
  await testPage.goto("/office/workspace/settings/export");
  await expect(testPage.getByText(/E2E Workspace export/)).toBeVisible({ timeout: 10_000 });
  await expect(testPage.getByText("kandev.yml", { exact: true })).toBeVisible();

  const downloadPromise = testPage.waitForEvent("download");
  await testPage.getByRole("button", { name: /Export \d+ files?/ }).click();
  const download = await downloadPromise;
  const archivePath = await download.path();
  expect(archivePath).toBeTruthy();

  const entries = readZipEntries(archivePath!);
  expect(entries.has(".kandev/kandev.yml")).toBe(true);
  expect(entries.get(".kandev/kandev.yml")?.toString("utf8")).toContain("name:");
});

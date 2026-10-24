import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  cpSync,
  mkdtempSync,
  readFileSync,
  readdirSync,
  rmSync,
  statSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

export function buildThemeRelease(id: string, version: string) {
  const dir = mkdtempSync(path.join(tmpdir(), "theme-package-"));
  try {
    const stage = path.join(dir, "sample");
    cpSync(
      path.resolve(__dirname, "../../../../backend/internal/plugins/themes/testdata/sample"),
      stage,
      { recursive: true },
    );
    const manifestPath = path.join(stage, "manifest.yaml");
    const manifest = readFileSync(manifestPath, "utf8")
      .replace(/^id:.*$/m, `id: ${id}`)
      .replace(/^version:.*$/m, `version: ${version}`);
    writeFileSync(manifestPath, manifest);
    const files = readdirSync(stage, { recursive: true })
      .filter(
        (file) =>
          statSync(path.join(stage, file)).isFile() &&
          file !== "checksums.txt" &&
          file !== "checksums.txt.sig",
      )
      .sort();
    const checksums = files.map(
      (file) =>
        `${createHash("sha256")
          .update(readFileSync(path.join(stage, file)))
          .digest("hex")}  ${file}`,
    );
    writeFileSync(path.join(stage, "checksums.txt"), checksums.join("\n") + "\n");
    const output = path.join(dir, "theme.tar.gz");
    execFileSync("tar", ["-czf", output, "-C", stage, ...files, "checksums.txt"]);
    const archive = readFileSync(output);
    return { version, archive, digest: createHash("sha256").update(archive).digest("hex") };
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { createRichOutputDescription } from "../lib/rich-output/guidance";
import { fileURLToPath } from "node:url";
import { createRichOutputToolSchema } from "../lib/rich-output/schema";

export function generateCatalog(target: string, check: boolean) {
  const schema = createRichOutputToolSchema();
  const artifacts = [
    { path: target, content: JSON.stringify(schema, null, 2) + "\n" },
    {
      path: join(dirname(target), "rich_output_catalog_v1.guidance.txt"),
      content: createRichOutputDescription(schema) + "\n",
    },
  ];
  if (check) {
    for (const artifact of artifacts) {
      if (readFileSync(artifact.path, "utf8") !== artifact.content) {
        // i18n-exempt: Build diagnostic, not browser copy.
        throw new Error("Rich-output catalog has drifted. Run pnpm rich-output:generate.");
      }
    }
  } else {
    for (const artifact of artifacts) writeFileSync(artifact.path, artifact.content);
  }
}
if (process.argv[1] === fileURLToPath(import.meta.url)) {
  generateCatalog(
    fileURLToPath(
      new URL(
        "../../backend/internal/mcp/server/rich_output_catalog_v1.schema.json",
        import.meta.url,
      ),
    ),
    process.argv.includes("--check"),
  );
}

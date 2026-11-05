import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const cataloguePath = fileURLToPath(
  new URL(
    "../../../backend/internal/agent/agents/managed_npm_runtime_versions.json",
    import.meta.url,
  ),
);

export function managedRuntimePackageSpec(packageName: string): string {
  const catalogue = JSON.parse(readFileSync(cataloguePath, "utf8")) as Record<string, string>;
  const version = catalogue[packageName];
  if (!version) {
    throw new Error(`managed runtime catalogue has no package ${packageName}`);
  }
  return `${packageName}@${version}`;
}

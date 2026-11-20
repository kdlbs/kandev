import { readFileSync } from "node:fs";
import path from "node:path";

const cataloguePath = path.resolve(
  __dirname,
  "../../../backend/internal/agent/agents/managed_npm_runtime_versions.json",
);

export function managedRuntimePackageSpec(packageName: string): string {
  const catalogue = JSON.parse(readFileSync(cataloguePath, "utf8")) as Record<string, string>;
  const version = catalogue[packageName];
  if (!version) {
    throw new Error(`managed runtime catalogue has no package ${packageName}`);
  }
  return `${packageName}@${version}`;
}

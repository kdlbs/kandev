import type { BuiltinMCPManifest, BuiltinMCPPackage } from "@/lib/types/builtin-mcp";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";

const PACKAGES: readonly BuiltinMCPPackage[] = [
  "core",
  "configuration",
  "rich-output",
  "walkthroughs",
  "diagnostics",
];
function record(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
function identity(value: unknown): value is string {
  return (
    typeof value === "string" && value.length > 0 && value.trim() === value && !value.includes("\0")
  );
}
function revision(value: unknown): boolean {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}
function eligible(id: BuiltinMCPPackage, surface: unknown): boolean {
  if (surface === "kanban-task") return true;
  if (surface === "office-task") return id === "core" || id === "rich-output";
  if (surface === "configuration") return id === "core" || id === "configuration";
  return false;
}
function validPackages(value: Record<string, unknown>): boolean {
  const packages = value.packages;
  const origins = value.origins;
  if (!Array.isArray(packages) || !record(origins) || !packages.includes("core")) return false;
  if (value.surface === "configuration" && !packages.includes("configuration")) return false;
  if (new Set(packages).size !== packages.length || Object.keys(origins).length !== packages.length)
    return false;
  return packages.every((id) => {
    if (!PACKAGES.includes(id) || !eligible(id, value.surface)) return false;
    if (id === "core" || value.surface === "configuration") return origins[id] === "required";
    return origins[id] === "task" || (origins[id] === "workspace" && id !== "configuration");
  });
}
export function isBuiltinMCPManifest(value: unknown): value is BuiltinMCPManifest {
  return (
    record(value) &&
    value.version === 1 &&
    value.catalog_version === 1 &&
    identity(value.task_id) &&
    identity(value.workspace_id) &&
    revision(value.task_revision) &&
    revision(value.workspace_revision) &&
    validPackages(value)
  );
}
export function validBuiltinAttempt(value: Record<string, unknown>): boolean {
  if (value.builtin_packages == null) return true;
  return (
    isBuiltinMCPManifest(value.builtin_packages) &&
    value.builtin_packages.task_id === value.task_id &&
    identity(value.session_id) &&
    identity(value.execution_id)
  );
}
export function validBuiltinAcceptance(
  value: unknown,
  current: { task_id?: string; session_id?: string },
): boolean {
  if (value == null) return true;
  return (
    record(value) &&
    isBuiltinMCPManifest(value.manifest) &&
    value.manifest.task_id === current.task_id &&
    value.session_id === current.session_id &&
    identity(value.session_id) &&
    identity(value.execution_id) &&
    identity(value.attachment_attempt_id) &&
    typeof value.accepted_at === "string" &&
    parseStrictRfc3339Timestamp(value.accepted_at) !== null
  );
}
export function validBuiltinServer(value: Record<string, unknown>): boolean {
  if (value.builtin_package == null) return true;
  if (!PACKAGES.includes(value.builtin_package as BuiltinMCPPackage)) return false;
  const name = value.builtin_package === "core" ? "kandev" : `kandev-${value.builtin_package}`;
  return value.source === "kandev" && value.name === name;
}

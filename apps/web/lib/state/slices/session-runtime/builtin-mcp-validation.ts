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
/**
 * Restrict packages to the execution surface's fixed allowlist.
 */
function eligible(id: BuiltinMCPPackage, surface: unknown): boolean {
  if (surface === "kanban-task" || surface === "automation") return true;
  if (surface === "office-task") return id === "core" || id === "rich-output";
  if (surface === "configuration") return id === "core" || id === "configuration";
  return false;
}
/**
 * Require unique supported packages, Core, and a valid origin for every selected package.
 */
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
    if (value.surface === "automation") return origins[id] === "automation";
    return origins[id] === "task" || (origins[id] === "workspace" && id !== "configuration");
  });
}
/**
 * Require automation identity only on version 2 automation manifests and reject it on legacy task
 * manifests.
 */
function validBindingVersion(value: Record<string, unknown>): boolean {
  if (value.surface !== "automation")
    return (
      value.version === 1 &&
      value.automation_id === undefined &&
      value.automation_revision === undefined &&
      value.automation_definition_ids === undefined
    );
  const ids = value.automation_definition_ids;
  return (
    value.version === 2 &&
    identity(value.automation_id) &&
    revision(value.automation_revision) &&
    Number(value.automation_revision) > 0 &&
    (ids === undefined ||
      (Array.isArray(ids) && ids.every(identity) && new Set(ids).size === ids.length))
  );
}
/**
 * Validate manifest identity, revision fields, package eligibility, and origins before accepting
 * runtime evidence.
 */
export function isBuiltinMCPManifest(value: unknown): value is BuiltinMCPManifest {
  return (
    record(value) &&
    validBindingVersion(value) &&
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

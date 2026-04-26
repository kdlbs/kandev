import type { ExportFile, FileTreeNode } from "./export-types";

/**
 * Serialize a config object to YAML-like key: value text.
 * Handles nested objects and arrays with simple indentation.
 */
function toYamlLike(obj: Record<string, unknown>, indent = 0): string {
  const pad = "  ".repeat(indent);
  const lines: string[] = [];

  for (const [key, value] of Object.entries(obj)) {
    if (value === undefined || value === null || value === "") continue;

    if (Array.isArray(value)) {
      lines.push(`${pad}${key}:`);
      for (const item of value) {
        if (typeof item === "object" && item !== null) {
          lines.push(`${pad}  -`);
          const nested = toYamlLike(item as Record<string, unknown>, indent + 2);
          lines.push(nested);
        } else {
          lines.push(`${pad}  - ${String(item)}`);
        }
      }
    } else if (typeof value === "object") {
      lines.push(`${pad}${key}:`);
      lines.push(toYamlLike(value as Record<string, unknown>, indent + 1));
    } else {
      lines.push(`${pad}${key}: ${String(value)}`);
    }
  }

  return lines.join("\n");
}

/**
 * Convert a raw export bundle (from the API) into a flat list of ExportFile entries.
 * Mirrors the zip structure produced by the Go backend.
 */
export function bundleToExportFiles(bundle: Record<string, unknown>): ExportFile[] {
  const files: ExportFile[] = [];
  const settings = bundle.settings as Record<string, unknown> | undefined;
  const agents = (bundle.agents as Record<string, unknown>[]) ?? [];
  const skills = (bundle.skills as Record<string, unknown>[]) ?? [];
  const routines = (bundle.routines as Record<string, unknown>[]) ?? [];
  const projects = (bundle.projects as Record<string, unknown>[]) ?? [];

  if (settings) {
    files.push({ path: "kandev.yml", content: toYamlLike(settings) });
  }

  for (const agent of agents) {
    const name = String(agent.name ?? "unnamed");
    files.push({ path: `agents/${name}.yml`, content: toYamlLike(agent) });
  }

  for (const skill of skills) {
    const slug = String(skill.slug ?? skill.name ?? "unnamed");
    files.push({ path: `skills/${slug}.yml`, content: toYamlLike(skill) });
  }

  for (const routine of routines) {
    const name = String(routine.name ?? "unnamed");
    files.push({ path: `routines/${name}.yml`, content: toYamlLike(routine) });
  }

  for (const project of projects) {
    const name = String(project.name ?? "unnamed");
    files.push({ path: `projects/${name}.yml`, content: toYamlLike(project) });
  }

  return files;
}

/** Build a tree structure from flat file paths. */
export function buildFileTree(files: ExportFile[]): FileTreeNode[] {
  const root: FileTreeNode[] = [];

  for (const file of files) {
    const parts = file.path.split("/");
    let current = root;

    for (let i = 0; i < parts.length; i++) {
      const part = parts[i];
      const isLast = i === parts.length - 1;
      const partialPath = parts.slice(0, i + 1).join("/");

      let node = current.find((n) => n.name === part);
      if (!node) {
        node = {
          name: part,
          path: partialPath,
          isDir: !isLast,
          children: [],
          content: isLast ? file.content : undefined,
        };
        current.push(node);
      }
      current = node.children;
    }
  }

  return sortTree(root);
}

function sortTree(nodes: FileTreeNode[]): FileTreeNode[] {
  return nodes
    .sort((a, b) => {
      if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
      return a.name.localeCompare(b.name);
    })
    .map((n) => ({ ...n, children: sortTree(n.children) }));
}

/** Count how many leaf files are in the selected set. */
export function countSelectedFiles(selected: Set<string>, files: ExportFile[]): number {
  return files.filter((f) => selected.has(f.path)).length;
}

/** Get all leaf file paths under a directory node. */
export function getDescendantFilePaths(node: FileTreeNode): string[] {
  if (!node.isDir) return [node.path];
  return node.children.flatMap(getDescendantFilePaths);
}

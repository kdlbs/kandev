export const INSPECTOR_SOURCE = "kandev-inspector" as const;

export interface CapturedElement {
  tag: string;
  id?: string;
  classes?: string;
  role?: string;
  ariaLabel?: string;
  text?: string;
  selector?: string;
}

export interface AnnotationRect {
  x: number;
  y: number;
  w: number;
  h: number;
}

export type AnnotationKind = "pin" | "area";

export interface Annotation {
  id: string;
  number: number;
  kind: AnnotationKind;
  pagePath: string;
  comment: string;
  rect?: AnnotationRect;
  element?: CapturedElement | null;
  elements?: CapturedElement[] | null;
}

/** Wire payload — number is assigned by the parent on receipt. */
export type AnnotationWirePayload = Omit<Annotation, "number">;

interface InspectorToggleCommand {
  source: typeof INSPECTOR_SOURCE;
  type: "toggle-inspect";
  payload: { active: boolean };
}

interface InspectorClearCommand {
  source: typeof INSPECTOR_SOURCE;
  type: "clear-annotations";
  payload: Record<string, never>;
}

interface InspectorRemoveMarkerCommand {
  source: typeof INSPECTOR_SOURCE;
  type: "remove-marker";
  payload: { number: number };
}

interface AnnotationAddedMessage {
  source: typeof INSPECTOR_SOURCE;
  type: "annotation-added";
  payload: AnnotationWirePayload;
}

interface InspectExitedMessage {
  source: typeof INSPECTOR_SOURCE;
  type: "inspect-exited";
  payload: Record<string, never>;
}

interface InspectorReadyMessage {
  source: typeof INSPECTOR_SOURCE;
  type: "inspector-ready";
  payload: Record<string, never>;
}

export type InspectorMessage =
  | AnnotationAddedMessage
  | InspectExitedMessage
  | InspectorReadyMessage;

export function isInspectorMessage(data: unknown): data is InspectorMessage {
  if (typeof data !== "object" || data === null) return false;
  const d = data as { source?: unknown; type?: unknown; payload?: unknown };
  return (
    d.source === INSPECTOR_SOURCE &&
    typeof d.type === "string" &&
    typeof d.payload === "object" &&
    d.payload !== null
  );
}

export function sendToggleInspect(iframe: HTMLIFrameElement, active: boolean): void {
  iframe.contentWindow?.postMessage(
    {
      source: INSPECTOR_SOURCE,
      type: "toggle-inspect",
      payload: { active },
    } satisfies InspectorToggleCommand,
    "*",
  );
}

export function sendClearAnnotations(iframe: HTMLIFrameElement): void {
  iframe.contentWindow?.postMessage(
    {
      source: INSPECTOR_SOURCE,
      type: "clear-annotations",
      payload: {},
    } satisfies InspectorClearCommand,
    "*",
  );
}

export function sendRemoveMarker(iframe: HTMLIFrameElement, number: number): void {
  iframe.contentWindow?.postMessage(
    {
      source: INSPECTOR_SOURCE,
      type: "remove-marker",
      payload: { number },
    } satisfies InspectorRemoveMarkerCommand,
    "*",
  );
}

function formatElement(el: CapturedElement): string {
  let idPart = "";
  if (el.id) idPart = `#${el.id}`;
  else if (el.classes) idPart = `.${el.classes.trim().split(/\s+/)[0]}`;
  return `\`${el.tag}${idPart}\``;
}

function formatPin(a: Annotation): string[] {
  const lines: string[] = [];
  const el = a.element;
  const role = el?.role ? ` [role="${el.role}"]` : "";
  let label = "";
  if (el?.ariaLabel) label = ` "${el.ariaLabel}"`;
  else if (el?.text) label = ` "${el.text}"`;
  const elPart = el ? ` ${formatElement(el)}` : "";
  lines.push(`${a.number}. [Pin]${elPart}${role}${label}`);
  if (el?.selector) lines.push(`   Selector: \`${el.selector}\``);
  if (a.comment) lines.push(`   Comment: ${a.comment}`);
  return lines;
}

function formatArea(a: Annotation): string[] {
  const lines: string[] = [];
  const r = a.rect;
  const header = r
    ? `${a.number}. [Area ${Math.round(r.w)}x${Math.round(r.h)} at (${Math.round(r.x)},${Math.round(r.y)})]`
    : `${a.number}. [Area]`;
  lines.push(header);
  if (a.elements && a.elements.length > 0) {
    const names = a.elements.map(formatElement).join(", ");
    lines.push(`   Contains: ${names}`);
  }
  if (a.comment) lines.push(`   Comment: ${a.comment}`);
  return lines;
}

export function formatAnnotations(annotations: Annotation[]): string {
  if (annotations.length === 0) return "";

  const byPath = new Map<string, Annotation[]>();
  for (const a of annotations) {
    const list = byPath.get(a.pagePath) ?? [];
    list.push(a);
    byPath.set(a.pagePath, list);
  }

  const sections: string[] = [];
  for (const [pagePath, list] of byPath) {
    const lines: string[] = [`> Preview annotations on \`${pagePath}\``, ">"];
    for (const a of list) {
      const body = a.kind === "pin" ? formatPin(a) : formatArea(a);
      for (const line of body) lines.push("> " + line);
      lines.push(">");
    }
    while (lines[lines.length - 1] === ">") lines.pop();
    sections.push(lines.join("\n"));
  }
  return sections.join("\n\n");
}

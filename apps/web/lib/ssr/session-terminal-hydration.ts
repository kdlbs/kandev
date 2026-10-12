import { fetchTerminals } from "@/lib/api/domains/user-shell-api";
import type { Terminal } from "@/hooks/domains/session/use-terminals";
import { t as translate } from "@/lib/i18n";

type TerminalApiResponse = Awaited<ReturnType<typeof fetchTerminals>>[number];

/** Whether a terminal from the API should be hydrated on SSR (skips placeholder and parked terminals). */
function shouldHydrateTerminal(t: TerminalApiResponse): boolean {
  const id = t.id ?? t.terminal_id ?? "";
  if (!id || id === "bottom-panel") return false;
  if (t.state === "parked") return false;
  return true;
}

/** Classifies a terminal as script and/or ordinary based on its kind, id prefix, and seq. */
function classifyTerminal(
  t: TerminalApiResponse,
  id: string,
): { isScript: boolean; isOrdinary: boolean } {
  const isScript = t.kind === "script" || id.startsWith("script-");
  const isOrdinary = t.kind === "ordinary" || (!isScript && t.seq !== undefined);
  return { isScript, isOrdinary };
}

/**
 * Derives the display label for a hydrated terminal: explicit names win, then a
 * numbered "Terminal N" for ordinary terminals, then script/terminal defaults.
 */
function deriveHydratedLabel(
  t: TerminalApiResponse,
  isScript: boolean,
  isOrdinary: boolean,
): string {
  if (t.display_name) return t.display_name;
  if (t.custom_name && t.custom_name !== "") return t.custom_name;
  if (t.label) return t.label;
  if (isOrdinary && t.seq) return translate("common:terminalNumbered", { seq: t.seq });
  return isScript ? translate("common:script") : translate("common:terminal");
}

/** Maps the classification flags to a terminal kind ("ordinary", "script", or undefined). */
function pickTerminalKind(
  isOrdinary: boolean,
  isScript: boolean,
): "ordinary" | "script" | undefined {
  if (isOrdinary) return "ordinary";
  if (isScript) return "script";
  return undefined;
}

/** Maps a terminal API response to the Terminal model used by the session page. */
function hydrateTerminal(t: TerminalApiResponse): Terminal {
  const id = (t.id ?? t.terminal_id ?? "") as string;
  const { isScript, isOrdinary } = classifyTerminal(t, id);
  const kind = pickTerminalKind(isOrdinary, isScript);
  return {
    id,
    type: isScript ? ("script" as const) : ("shell" as const),
    label: deriveHydratedLabel(t, isScript, isOrdinary),
    closable: t.closable ?? true,
    kind,
    seq: t.seq,
    customName: t.custom_name ?? undefined,
    state: t.state,
    ptyStatus: t.pty_status,
  };
}
export function hydrateSessionTerminals(terminals: TerminalApiResponse[]): Terminal[] {
  return terminals.filter(shouldHydrateTerminal).map(hydrateTerminal);
}

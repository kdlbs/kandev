"use client";

import { useEffect, useRef, type RefObject } from "react";
import { usePathname, useRouter, useSearchParams } from "@/lib/routing/client-router";
import { runWithNavigationBlockerBypassed } from "@/lib/routing/navigation-guard";
import { removeMCPAssignmentHint, resolveMCPAssignmentHint } from "@/lib/mcp/assignment-hint";

const HIGHLIGHT_ATTRIBUTE = "data-mcp-highlighted";

type FocusInput = {
  container: RefObject<HTMLElement | null>;
  ownerKey: string;
  workspaceId: string;
  definitions: ReadonlyArray<{ id: string; workspace_id: string; enabled: boolean }>;
  active: boolean;
  ready: boolean;
};

export function useMCPAssignmentFocus({
  container,
  ownerKey,
  workspaceId,
  definitions,
  active,
  ready,
}: FocusInput) {
  const search = useSearchParams().toString();
  const pathname = usePathname();
  const router = useRouter();
  const consumed = useRef<string | null>(null);
  const highlighted = useRef<HTMLElement | null>(null);
  const context = JSON.stringify([ownerKey, workspaceId]);
  const lastContext = useRef(context);
  useEffect(() => {
    if (lastContext.current !== context) {
      consumed.current = null;
      lastContext.current = context;
    }
    return () => {
      highlighted.current?.removeAttribute(HIGHLIGHT_ATTRIBUTE);
      highlighted.current = null;
    };
  }, [context]);
  useEffect(() => {
    const rows = Array.from(
      container.current?.querySelectorAll<HTMLElement>("[data-mcp-definition-id]") ?? [],
    );
    const resolution = resolveMCPAssignmentHint({
      search,
      workspaceId,
      definitions,
      active,
      ready,
      visibleDefinitionIds: rows.map((row) => row.dataset.mcpDefinitionId ?? ""),
    });
    if (resolution.status === "absent") {
      consumed.current = null;
      return;
    }
    if (resolution.status !== "consume") return;
    if (consumed.current === search) {
      const row = rows.find((item) => item.dataset.mcpDefinitionId === resolution.definitionId);
      row?.setAttribute(HIGHLIGHT_ATTRIBUTE, "true");
      highlighted.current = row ?? null;
      return;
    }
    consumed.current = search;
    highlighted.current?.removeAttribute(HIGHLIGHT_ATTRIBUTE);
    const row = rows.find((item) => item.dataset.mcpDefinitionId === resolution.definitionId);
    highlighted.current = row ?? null;
    if (row) {
      row.setAttribute(HIGHLIGHT_ATTRIBUTE, "true");
      row.scrollIntoView?.({ block: "nearest" });
      row.focus({ preventScroll: true });
    }
    const href = removeMCPAssignmentHint(`${pathname}?${search}${window.location.hash}`);
    runWithNavigationBlockerBypassed(() => router.replace(href, { scroll: false }));
  }, [container, ownerKey, workspaceId, definitions, active, ready, pathname, search, router]);
}

"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";
import { LOCATION_CHANGE_EVENT } from "@/lib/routing/navigation-event";
import {
  SETTINGS_TARGET_REQUEST_EVENT,
  settingsTargetFromHash,
  type SettingsTargetRequestDetail,
} from "@/lib/settings-discovery/target";

/**
 * Expansion state for one workflow card in the Workflows settings list.
 * Detailed view is always open. Compact view starts collapsed unless the
 * workflow is a draft, and once opened the body stays mounted so local editor
 * state survives collapsing.
 */
export function useWorkflowCardDisclosure({
  compact,
  initiallyExpanded,
  targetId,
}: {
  compact: boolean;
  initiallyExpanded: boolean;
  /** Settings target that expands the card when navigated to. */
  targetId?: string;
}) {
  const [expanded, setExpanded] = useState(initiallyExpanded);
  const [hasOpened, setHasOpened] = useState(initiallyExpanded);
  const open = !compact || expanded;
  // Any render that shows the body (including Detailed view) keeps it mounted
  // afterwards, so switching to Compact hides it instead of unmounting it.
  if (open && !hasOpened) setHasOpened(true);

  const toggle = () => {
    setExpanded((previous) => !previous);
    setHasOpened(true);
  };

  const expand = () => {
    setExpanded(true);
    setHasOpened(true);
  };

  useExpandOnSettingsTarget(targetId, expand);

  return {
    expanded,
    open,
    mounted: open || hasOpened,
    toggle,
    expand,
    discoveryTargetId: open ? targetId : undefined,
  };
}

export function WorkflowCardDisclosureBody({
  id,
  open,
  mounted,
  children,
}: {
  id: string;
  open: boolean;
  mounted: boolean;
  children: ReactNode;
}) {
  if (!mounted) return <div id={id} hidden />;
  return (
    <div id={id} hidden={!open}>
      {children}
    </div>
  );
}

/**
 * Calls `expand` whenever settings navigation targets `targetId`: a matching
 * URL fragment on mount or navigation, or a settings-search target request.
 */
export function useExpandOnSettingsTarget(targetId: string | undefined, expand: () => void) {
  const expandRef = useRef(expand);
  expandRef.current = expand;

  useEffect(() => {
    if (!targetId) return;
    const expandOnHash = () => {
      if (settingsTargetFromHash(window.location.hash) === targetId) expandRef.current();
    };
    const expandOnRequest = (event: Event) => {
      const requested = (event as CustomEvent<SettingsTargetRequestDetail>).detail?.targetId;
      if (requested === targetId) expandRef.current();
    };
    window.addEventListener(LOCATION_CHANGE_EVENT, expandOnHash);
    window.addEventListener("hashchange", expandOnHash);
    window.addEventListener("popstate", expandOnHash);
    window.addEventListener(SETTINGS_TARGET_REQUEST_EVENT, expandOnRequest);
    expandOnHash();
    return () => {
      window.removeEventListener(LOCATION_CHANGE_EVENT, expandOnHash);
      window.removeEventListener("hashchange", expandOnHash);
      window.removeEventListener("popstate", expandOnHash);
      window.removeEventListener(SETTINGS_TARGET_REQUEST_EVENT, expandOnRequest);
    };
  }, [targetId]);
}

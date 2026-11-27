import { useLayoutEffect, useRef, useState, type FocusEvent } from "react";
import type { StorageResource } from "./storage-overview-resources";
import {
  SYSTEM_TEMPORARY_RESOURCE_ID,
  TEMPORARY_ARTIFACTS_RESOURCE_ID,
} from "./storage-overview-resources";

interface FocusedStorageTarget {
  resourceId: string;
  element: HTMLElement;
  focusId: string;
}

function focusedElementAfterReorder(
  container: HTMLElement,
  target: FocusedStorageTarget,
): HTMLElement | null {
  const resource = Array.from(
    container.querySelectorAll<HTMLElement>("[data-storage-resource-id]"),
  ).find((element) => element.dataset.storageResourceId === target.resourceId);
  if (!resource) return null;
  return (
    Array.from(resource.querySelectorAll<HTMLElement>("[data-storage-focus-id]")).find(
      (element) => element.dataset.storageFocusId === target.focusId,
    ) ?? null
  );
}

function rememberStorageFocus(
  event: FocusEvent<HTMLDivElement>,
  resourcesRef: { current: HTMLDivElement | null },
  focusedTargetRef: { current: FocusedStorageTarget | null },
) {
  const target = event.target;
  if (!(target instanceof HTMLElement)) return;
  const container = resourcesRef.current;
  const resource = target.closest<HTMLElement>("[data-storage-resource-id]");
  const focusId = target.dataset.storageFocusId;
  if (!container || !resource || !container.contains(resource) || !focusId) {
    focusedTargetRef.current = null;
    return;
  }
  focusedTargetRef.current = {
    resourceId: resource.dataset.storageResourceId ?? "",
    element: target,
    focusId,
  };
}

function forgetStorageFocusOutsideResources(
  event: FocusEvent<HTMLDivElement>,
  resourcesRef: { current: HTMLDivElement | null },
  focusedTargetRef: { current: FocusedStorageTarget | null },
) {
  const relatedTarget = event.relatedTarget;
  if (!(relatedTarget instanceof Node) || !resourcesRef.current?.contains(relatedTarget)) {
    focusedTargetRef.current = null;
  }
}

export function useStorageOverviewFocus({
  resources,
  focusTemporaryEntries,
  focusTemporaryCleanup,
}: {
  resources: StorageResource[];
  focusTemporaryEntries?: number;
  focusTemporaryCleanup?: number;
}) {
  const resourcesRef = useRef<HTMLDivElement | null>(null);
  const focusedTargetRef = useRef<FocusedStorageTarget | null>(null);
  const lastTemporaryFocusRequest = useRef(0);
  const lastTemporaryCleanupFocusRequest = useRef(0);
  const pendingFocusResource = useRef<string | null>(null);
  const [expandedResources, setExpandedResources] = useState<string[]>([]);
  const resourceOrderKey = resources.map((resource) => resource.id).join("\u0000");

  useLayoutEffect(() => {
    if (!focusTemporaryEntries || focusTemporaryEntries === lastTemporaryFocusRequest.current)
      return;
    lastTemporaryFocusRequest.current = focusTemporaryEntries;
    pendingFocusResource.current = SYSTEM_TEMPORARY_RESOURCE_ID;
    setExpandedResources((current) =>
      current.includes(SYSTEM_TEMPORARY_RESOURCE_ID)
        ? current
        : [...current, SYSTEM_TEMPORARY_RESOURCE_ID],
    );
  }, [focusTemporaryEntries]);

  useLayoutEffect(() => {
    if (
      !focusTemporaryCleanup ||
      focusTemporaryCleanup === lastTemporaryCleanupFocusRequest.current
    ) {
      return;
    }
    lastTemporaryCleanupFocusRequest.current = focusTemporaryCleanup;
    pendingFocusResource.current = TEMPORARY_ARTIFACTS_RESOURCE_ID;
    setExpandedResources((current) =>
      current.includes(TEMPORARY_ARTIFACTS_RESOURCE_ID)
        ? current
        : [...current, TEMPORARY_ARTIFACTS_RESOURCE_ID],
    );
  }, [focusTemporaryCleanup]);

  useLayoutEffect(() => {
    const resourceId = pendingFocusResource.current;
    if (!resourceId || !expandedResources.includes(resourceId)) return;
    const trigger = resourcesRef.current?.querySelector<HTMLElement>(
      `[data-storage-resource-id="${resourceId}"] [data-storage-focus-id="trigger"]`,
    );
    if (trigger && typeof trigger.scrollIntoView === "function") {
      trigger.scrollIntoView({ block: "nearest" });
    }
    trigger?.focus();
    pendingFocusResource.current = null;
  }, [expandedResources, focusTemporaryEntries, focusTemporaryCleanup]);

  useLayoutEffect(() => {
    const container = resourcesRef.current;
    const target = focusedTargetRef.current;
    if (!container || !target) return;
    const activeElement = document.activeElement;
    if (
      activeElement &&
      activeElement !== document.body &&
      (!container.contains(activeElement) || activeElement !== target.element)
    ) {
      focusedTargetRef.current = null;
      return;
    }
    const element = focusedElementAfterReorder(container, target);
    if (element && document.activeElement !== element) element.focus();
    focusedTargetRef.current = null;
  }, [resourceOrderKey]);

  const rememberFocusedTarget = (event: FocusEvent<HTMLDivElement>) =>
    rememberStorageFocus(event, resourcesRef, focusedTargetRef);
  const forgetFocusedTargetOutsideResources = (event: FocusEvent<HTMLDivElement>) =>
    forgetStorageFocusOutsideResources(event, resourcesRef, focusedTargetRef);

  return {
    resourcesRef,
    expandedResources,
    setExpandedResources,
    rememberFocusedTarget,
    forgetFocusedTargetOutsideResources,
  };
}

import { useSyncExternalStore } from "react";

export type WalkthroughViewportRect = {
  left: number;
  top: number;
  right: number;
  bottom: number;
  width: number;
  height: number;
};

export type WalkthroughEditorAnchor = {
  key: string;
  taskId: string;
  stepIndex: number;
  file: string;
  repo?: string;
  line: number;
  lineEnd: number;
  rect: WalkthroughViewportRect;
  container?: HTMLElement;
};

type Listener = () => void;
type ElementFromPoint = (x: number, y: number) => Element | null;

let currentAnchor: WalkthroughEditorAnchor | null = null;
const listeners = new Set<Listener>();

function emit() {
  for (const listener of listeners) listener();
}

export function setWalkthroughEditorAnchor(anchor: WalkthroughEditorAnchor | null): void {
  currentAnchor = anchor;
  emit();
}

export function clearWalkthroughEditorAnchor(key: string): void {
  if (currentAnchor?.key !== key) return;
  currentAnchor = null;
  emit();
}

export function getWalkthroughEditorAnchor(): WalkthroughEditorAnchor | null {
  return currentAnchor;
}

export function subscribeWalkthroughEditorAnchor(listener: Listener): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function useWalkthroughEditorAnchor(): WalkthroughEditorAnchor | null {
  return useSyncExternalStore(
    subscribeWalkthroughEditorAnchor,
    getWalkthroughEditorAnchor,
    getWalkthroughEditorAnchor,
  );
}

function center(rect: WalkthroughViewportRect) {
  return {
    x: rect.left + rect.width / 2,
    y: rect.top + rect.height / 2,
  };
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

function rectIntersectsViewport(rect: WalkthroughViewportRect): boolean {
  if (rect.width <= 0 || rect.height <= 0) return false;
  if (typeof window === "undefined") return true;
  return (
    rect.right > 0 &&
    rect.bottom > 0 &&
    rect.left < window.innerWidth &&
    rect.top < window.innerHeight
  );
}

export function isWalkthroughAnchorTargetVisible(
  container: HTMLElement | null,
  rect: WalkthroughViewportRect,
  elementFromPoint: ElementFromPoint = (x, y) => document.elementFromPoint(x, y),
): boolean {
  if (!container?.isConnected || !rectIntersectsViewport(rect)) return false;
  const containerRect = container.getBoundingClientRect();
  if (containerRect.width <= 0 || containerRect.height <= 0) return false;
  const style = window.getComputedStyle(container);
  if (style.display === "none" || style.visibility === "hidden") return false;

  const point = center(rect);
  if (point.x < 0 || point.y < 0 || point.x > window.innerWidth || point.y > window.innerHeight) {
    return false;
  }
  const elementAtPoint = elementFromPoint(point.x, point.y);
  return !!elementAtPoint && container.contains(elementAtPoint);
}

export function computeWalkthroughConnectorPath(
  card: WalkthroughViewportRect,
  anchor: WalkthroughViewportRect,
): string | null {
  if (card.width <= 0 || card.height <= 0 || anchor.width <= 0 || anchor.height <= 0) return null;

  const cardCenter = center(card);
  const anchorCenter = center(anchor);
  const cardIsLeft = cardCenter.x < anchorCenter.x;
  const source = {
    x: cardIsLeft ? card.right : card.left,
    y: clamp(anchorCenter.y, card.top + 24, card.bottom - 24),
  };
  const target = {
    x: cardIsLeft ? anchor.left : anchor.right,
    y: anchorCenter.y,
  };
  const direction = cardIsLeft ? 1 : -1;
  const curve = Math.max(48, Math.min(220, Math.abs(target.x - source.x) * 0.45));

  return [
    `M ${source.x.toFixed(1)} ${source.y.toFixed(1)}`,
    `C ${(source.x + direction * curve).toFixed(1)} ${source.y.toFixed(1)}`,
    `${(target.x - direction * curve).toFixed(1)} ${target.y.toFixed(1)}`,
    `${target.x.toFixed(1)} ${target.y.toFixed(1)}`,
  ].join(" ");
}

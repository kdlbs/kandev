/**
 * Pure decision logic for the transcript auto-scroll toggle, shared by the
 * native and Virtuoso message list renderers so the two stay behaviorally
 * consistent. Kept side-effect free for direct unit testing — the renderers
 * own all DOM / Virtuoso imperative calls and just consult these functions.
 */

/**
 * Distinguishes a genuine prepend (older messages loaded above the current
 * view, e.g. via lazy-loading) from a plain append (a new message arriving
 * at the end) when the rendered item count grows. `useScrollPositionOnPrepend`
 * only needs to compensate scrollTop for the former — compensating an append
 * too would drag a scrolled-away user's view down by the height of every new
 * message, fighting the auto-scroll toggle's "stay put while disabled"
 * contract (and, independently of that toggle, jittering anyone who has
 * scrolled up to read older messages while new ones arrive).
 */
export function isPrependUpdate(params: {
  prevItemCount: number;
  nextItemCount: number;
  prevFirstKey: string | null;
  nextFirstKey: string | null;
}): boolean {
  if (params.nextItemCount <= params.prevItemCount) return false;
  if (params.nextFirstKey === null) return false;
  return params.nextFirstKey !== params.prevFirstKey;
}

/**
 * Whether new messages arriving should force-scroll the transcript to the
 * bottom. The native renderer already gates this on `isNearBottom`; auto-scroll
 * being disabled overrides that and suppresses the jump entirely.
 */
export function shouldAutoScrollOnMessagesChange(enabled: boolean, isNearBottom: boolean): boolean {
  return enabled && isNearBottom;
}

/**
 * Whether the agent starting a turn (isWorking transitioning to true) should
 * force-scroll to the bottom. Disabled auto-scroll suppresses this too — while
 * off, nothing should move the view.
 */
export function shouldAutoScrollOnWorkingStart(enabled: boolean): boolean {
  return enabled;
}

// Matches the settle tolerance used elsewhere for "has this scrolled past
// view" checks (see the last-prompt scroll-nav feature) — sub-pixel jitter
// from layout rounding shouldn't count as "progressed".
const SETTLE_TOLERANCE_PX = 2;

/**
 * Whether the transcript's content bottom currently sits meaningfully below
 * the visible viewport bottom, i.e. there is unseen content past the current
 * scroll position.
 */
export function hasTranscriptProgressedPastView(params: {
  scrollTop: number;
  scrollHeight: number;
  clientHeight: number;
}): boolean {
  return params.scrollHeight - params.scrollTop - params.clientHeight > SETTLE_TOLERANCE_PX;
}

/**
 * Whether re-enabling auto-scroll (the user flips the toggle back on) should
 * immediately catch the view up to the bottom. Only fires on the disabled ->
 * enabled transition, and only when the transcript has actually progressed
 * past the current view while it was disabled — resuming auto-follow with no
 * new content, or while the user is already at the bottom, never forces a
 * scroll here.
 */
export function shouldCatchUpOnAutoScrollEnable(params: {
  wasEnabled: boolean;
  nowEnabled: boolean;
  isAtBottom: boolean;
}): boolean {
  if (params.wasEnabled || !params.nowEnabled) return false;
  return !params.isAtBottom;
}

/**
 * Resolves the scrollTop the native list should apply right after mount
 * (session opened, or the panel remounted via a dockview layout rebuild).
 *
 * - A pending dockview layout-rebuild restore always wins; that separate
 *   mechanism owns the position for maximize/un-maximize and runs
 *   independently of this feature.
 * - When auto-scroll is disabled, restore whatever offset was captured the
 *   last time this session's transcript was visible — falling back to the
 *   bottom only if nothing was ever captured (first-time disable, or a
 *   session that has never been scrolled).
 * - When enabled, the pre-existing behavior (scroll to bottom) applies.
 *
 * Returns `null` when the caller should skip applying any scrollTop.
 */
export function resolveNativeInitialScrollTop(params: {
  enabled: boolean;
  hasPendingLayoutRestore: boolean;
  savedScrollTop: number | undefined;
  scrollHeight: number;
}): number | null {
  if (params.hasPendingLayoutRestore) return null;
  if (!params.enabled) return params.savedScrollTop ?? params.scrollHeight;
  return params.scrollHeight;
}

const FOLLOW_SMOOTH = "smooth" as const;

/**
 * Virtuoso `followOutput` resolver: identical to the renderer's original
 * always-on behavior, except disabled auto-scroll unconditionally suppresses
 * following regardless of Virtuoso's own atBottom tracking.
 */
export function resolveFollowOutput(enabled: boolean, isAtBottom: boolean): "smooth" | false {
  if (!enabled) return false;
  return isAtBottom ? FOLLOW_SMOOTH : false;
}

import { useCallback, useEffect, useRef } from 'react';

type HunkHoverCallbacks = {
  wrapperRef: React.RefObject<HTMLDivElement | null>;
  changeLineMapRef: React.RefObject<Map<string, string>>;
  hideTimeoutRef: React.MutableRefObject<ReturnType<typeof setTimeout> | null>;
};

type HunkHoverResult = {
  onLineEnter: (props: { lineType?: string; lineNumber?: number; annotationSide?: string }) => void;
  onLineLeave: () => void;
  onButtonEnter: () => void;
  onButtonLeave: () => void;
};

/**
 * Manages show/hide of undo buttons when hovering change lines in the diff viewer.
 * Uses direct DOM manipulation to avoid re-renders.
 */
export function useHunkHover({ wrapperRef, changeLineMapRef, hideTimeoutRef }: HunkHoverCallbacks): HunkHoverResult {
  const activeBlockRef = useRef<string | null>(null);
  const isHoveringButtonRef = useRef(false);

  const setBlockVisible = useCallback((cbId: string | null, visible: boolean) => {
    if (!cbId) return;
    const btn = wrapperRef.current?.querySelector(`[data-cb="${cbId}"] [data-undo-btn]`);
    if (btn instanceof HTMLElement) {
      btn.style.opacity = visible ? '1' : '0';
      btn.style.pointerEvents = visible ? 'auto' : 'none';
    }
  }, [wrapperRef]);

  const showBlock = useCallback((cbId: string) => {
    if (hideTimeoutRef.current) { clearTimeout(hideTimeoutRef.current); hideTimeoutRef.current = null; }
    if (activeBlockRef.current === cbId) return;
    setBlockVisible(activeBlockRef.current, false);
    activeBlockRef.current = cbId;
    setBlockVisible(cbId, true);
  }, [setBlockVisible, hideTimeoutRef]);

  const hideBlock = useCallback(() => {
    if (hideTimeoutRef.current) clearTimeout(hideTimeoutRef.current);
    hideTimeoutRef.current = setTimeout(() => {
      if (isHoveringButtonRef.current) return;
      setBlockVisible(activeBlockRef.current, false);
      activeBlockRef.current = null;
    }, 200);
  }, [setBlockVisible, hideTimeoutRef]);

  const onButtonEnter = useCallback(() => {
    isHoveringButtonRef.current = true;
    if (hideTimeoutRef.current) { clearTimeout(hideTimeoutRef.current); hideTimeoutRef.current = null; }
  }, [hideTimeoutRef]);

  const onButtonLeave = useCallback(() => {
    isHoveringButtonRef.current = false;
    hideBlock();
  }, [hideBlock]);

  const showBlockRef = useRef(showBlock);
  const hideBlockRef = useRef(hideBlock);
  useEffect(() => { showBlockRef.current = showBlock; }, [showBlock]);
  useEffect(() => { hideBlockRef.current = hideBlock; }, [hideBlock]);

  const onLineEnter = useCallback((props: { lineType?: string; lineNumber?: number; annotationSide?: string }) => {
    const { lineType, lineNumber, annotationSide } = props;
    if (!lineType?.startsWith('change-') || lineNumber == null) { hideBlockRef.current(); return; }
    const side = lineType === 'change-deletion' ? 'deletions' : 'additions';
    const key = `${annotationSide ?? side}:${lineNumber}`;
    const cbId = changeLineMapRef.current.get(key);
    if (cbId) showBlockRef.current(cbId);
    else hideBlockRef.current();
  }, [changeLineMapRef]);

  const onLineLeave = useCallback(() => { hideBlockRef.current(); }, []);

  return { onLineEnter, onLineLeave, onButtonEnter, onButtonLeave };
}

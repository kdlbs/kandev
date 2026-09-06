import {
  type Dispatch,
  type RefCallback,
  type SetStateAction,
  useCallback,
  useLayoutEffect,
  useRef,
  useState,
} from "react";

type QueuedMessageOverflow = {
  previewRef: RefCallback<HTMLDivElement>;
  disclosureButtonRef: RefCallback<HTMLButtonElement>;
  canExpand: boolean;
};

/** Measures a collapsed queue preview at the width available without its optional disclosure. */
export function useQueuedMessageOverflow(
  visible: string,
  expanded: boolean,
  setExpanded: Dispatch<SetStateAction<boolean>>,
): QueuedMessageOverflow {
  const [previewElement, setPreviewElement] = useState<HTMLDivElement | null>(null);
  const [canExpand, setCanExpand] = useState(false);
  const previewIdentityRef = useRef<HTMLDivElement | null>(null);
  const disclosureIdentityRef = useRef<HTMLButtonElement | null>(null);
  const collapsedCapRef = useRef(0);
  const generationRef = useRef(0);

  const previewRef = useCallback<RefCallback<HTMLDivElement>>((element) => {
    previewIdentityRef.current = element;
    setPreviewElement(element);
  }, []);
  const disclosureButtonRef = useCallback<RefCallback<HTMLButtonElement>>((element) => {
    disclosureIdentityRef.current = element;
  }, []);

  useLayoutEffect(() => {
    const generation = generationRef.current + 1;
    generationRef.current = generation;
    const preview = previewElement;
    const isCurrent = () =>
      generationRef.current === generation && previewIdentityRef.current === preview;

    if (!preview || visible.length === 0) {
      collapsedCapRef.current = 0;
      setCanExpand(false);
      setExpanded(false);
      return () => {
        if (generationRef.current === generation) generationRef.current += 1;
      };
    }

    const measure = () => {
      if (!isCurrent()) return;
      const disclosure = disclosureIdentityRef.current;
      const previousDisplay = disclosure?.style.display;
      let scrollHeight: number;
      let clientHeight: number;
      try {
        if (disclosure) disclosure.style.display = "none";
        scrollHeight = preview.scrollHeight;
        clientHeight = preview.clientHeight;
      } finally {
        if (disclosure) disclosure.style.display = previousDisplay ?? "";
      }
      if (!isCurrent()) return;

      if (expanded) {
        const overflowsCollapsedCap = scrollHeight > collapsedCapRef.current;
        setCanExpand(overflowsCollapsedCap);
        if (!overflowsCollapsedCap) setExpanded(false);
        return;
      }

      collapsedCapRef.current = clientHeight;
      setCanExpand(scrollHeight > clientHeight);
    };

    const handleWindowResize = () => measure();
    const visualViewport = window.visualViewport;
    const handleAsyncContent = () => measure();
    const fonts = document.fonts;

    preview.addEventListener("load", handleAsyncContent, true);
    fonts?.addEventListener("loadingdone", handleAsyncContent);
    void fonts?.ready.then(handleAsyncContent);

    measure();
    window.addEventListener("resize", handleWindowResize);
    visualViewport?.addEventListener("resize", handleWindowResize);

    const ResizeObserverConstructor = window.ResizeObserver;
    const observer =
      typeof ResizeObserverConstructor === "function"
        ? new ResizeObserverConstructor((entries) => {
            if (!isCurrent() || !entries.some((entry) => entry.target === preview)) return;
            measure();
          })
        : null;
    observer?.observe(preview);

    return () => {
      if (generationRef.current === generation) generationRef.current += 1;
      window.removeEventListener("resize", handleWindowResize);
      visualViewport?.removeEventListener("resize", handleWindowResize);
      preview.removeEventListener("load", handleAsyncContent, true);
      fonts?.removeEventListener("loadingdone", handleAsyncContent);
      observer?.disconnect();
    };
  }, [expanded, previewElement, setExpanded, visible]);

  return { previewRef, disclosureButtonRef, canExpand };
}

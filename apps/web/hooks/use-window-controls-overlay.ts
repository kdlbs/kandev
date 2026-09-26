import { useEffect, useState } from "react";
import { isMacTauriWebview } from "@/lib/desktop/window-chrome";

export type WindowControlsOverlayState = {
  visible: boolean;
  x: number;
  y: number;
  width: number;
  height: number;
  macTauriOverlay: boolean;
};

const INACTIVE_OVERLAY: WindowControlsOverlayState = {
  visible: false,
  x: 0,
  y: 0,
  width: 0,
  height: 0,
  macTauriOverlay: false,
};

function readWindowControlsOverlay(): WindowControlsOverlayState {
  const macTauriOverlay = isMacTauriWebview();
  if (macTauriOverlay) {
    return { visible: false, x: 0, y: 0, width: 0, height: 0, macTauriOverlay: true };
  }
  const overlay = navigator.windowControlsOverlay;
  if (!overlay?.visible) return INACTIVE_OVERLAY;

  const { x, y, width, height } = overlay.getTitlebarAreaRect();
  return { visible: true, x, y, width, height, macTauriOverlay: false };
}

export function useWindowControlsOverlay(): WindowControlsOverlayState {
  const [state, setState] = useState(readWindowControlsOverlay);

  useEffect(() => {
    const overlay = navigator.windowControlsOverlay;
    if (!overlay || isMacTauriWebview()) return;

    const handleGeometryChange = () => setState(readWindowControlsOverlay());
    overlay.addEventListener("geometrychange", handleGeometryChange);
    handleGeometryChange();
    return () => overlay.removeEventListener("geometrychange", handleGeometryChange);
  }, []);

  return state;
}

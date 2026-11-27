"use client";

import type { RefObject } from "react";
import { ZoomCluster } from "./review-desktop-controls";
import { ZoomChip } from "./review-phone-controls";
import type { ChangesStatus } from "./use-changes-zoom";
import type { useReviewViewport } from "./use-review-viewport";

type Props = {
  viewport: ReturnType<typeof useReviewViewport>;
  phone: boolean;
  container: RefObject<HTMLElement | null>;
  changes?: { status: ChangesStatus; onClick: () => void };
};

/** Zoom on the image: a chip that opens the controls on phones, a cluster on desktop. */
export function StageZoom({ viewport, phone, container, changes }: Props) {
  if (phone) {
    return (
      <ZoomChip
        scale={viewport.view.scale}
        onZoom={viewport.zoomStep}
        onFit={viewport.fit}
        onActual={viewport.actual}
        container={container}
      />
    );
  }
  return (
    <ZoomCluster
      scale={viewport.view.scale}
      onZoom={viewport.zoomStep}
      onFit={viewport.fit}
      onScale={viewport.zoomTo}
      changes={changes}
      container={container}
    />
  );
}

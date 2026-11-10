"use client";

import { IconArrowsExchange } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { ReviewCanvas, ReviewImage } from "./review-canvas";
import { CandidateStepper, PinnedWipeLine } from "./review-compare-controls";
import { ChangesButton, StageOverlayControl, ZoomCluster } from "./review-desktop-controls";
import { CompareLayers, WipeHandle, type LoadedReviewImage } from "./review-compare-layers";
import { ReviewCompareStatus } from "./review-compare-status";
import { ModeControls, ReviewCompareToolbar } from "./review-compare-toolbar";
import { ReviewBottomBar } from "./review-layout";
import {
  ImagePickerSheetButton,
  ModeSheetButton,
  MoreSheetButton,
  StatusChips,
  ZoomChip,
} from "./review-phone-controls";
import { RegionsButton } from "./review-region-panel";
import { ImageState, stopStageKeys } from "./review-stage-shared";
import { ReviewZoomControls, reviewControlSize } from "./review-zoom-controls";
import {
  useCompareStage,
  type CompareStageModel,
  type CompareStageProps,
} from "./use-compare-stage";

export { unionSize } from "./use-compare-stage";

/** Compares a baseline (A) with a candidate (B) on bytes already loaded for the segment. */
export function ReviewCompareStage(props: CompareStageProps) {
  const { t } = useTranslation();
  const model = useCompareStage(props);
  return (
    <section
      className="flex min-h-0 flex-1 flex-col gap-2"
      aria-label={t("task:questionReviewStage")}
      onKeyDown={stopStageKeys}
    >
      {model.layout.phone ? <PhoneTopRow model={model} /> : <DesktopToolbar model={model} />}
      <CanvasArea model={model} />
      {model.layout.phone && <PhoneBottomBar model={model} />}
      {model.regions.popover}
    </section>
  );
}

function DesktopToolbar({ model }: { model: CompareStageModel }) {
  return (
    <ReviewCompareToolbar
      compare={model.compare}
      images={model.segment.images}
      wipe={model.wipe}
      urls={model.urls}
    />
  );
}

function PhoneTopRow({ model }: { model: CompareStageModel }) {
  return (
    <div className="flex items-center gap-2">
      <ModeSheetButton
        mode={model.state.mode}
        onChange={(mode) => model.compare.dispatch({ type: "mode", mode })}
      />
      {model.a && model.b && (
        <ChangesButton status={model.changes.status} onClick={model.changes.trigger} />
      )}
      <div className="ml-auto flex items-center gap-2">
        {(model.marking || model.annotations.list(model.segment.questionId).length > 0) && (
          <RegionsButton controller={model.annotations} questionId={model.segment.questionId} />
        )}
        <MoreSheetButton>
          {(close) => (
            <>
              <ReviewZoomControls
                scale={model.viewport.view.scale}
                onZoom={model.viewport.zoomStep}
                onFit={model.viewport.fit}
                onActual={model.viewport.actual}
              />
              <div onClickCapture={close}>{model.regions.button}</div>
            </>
          )}
        </MoreSheetButton>
      </div>
    </div>
  );
}

function SwapButton({ model }: { model: CompareStageModel }) {
  const { t } = useTranslation();
  return (
    <Button
      type="button"
      variant="outline"
      size="icon"
      className="h-11 w-11 shrink-0 cursor-pointer"
      aria-label={t("task:questionReviewSwap")}
      data-testid="question-review-swap"
      onClick={() => model.compare.dispatch({ type: "swap" })}
    >
      <IconArrowsExchange className="h-4 w-4" aria-hidden="true" />
    </Button>
  );
}

function PhoneBottomBar({ model }: { model: CompareStageModel }) {
  const { t } = useTranslation();
  const { state, compare } = model;
  const stepping = state.mode === "toggle" || state.mode === "side";
  return (
    <ReviewBottomBar>
      <SwapButton model={model} />
      {stepping && model.canStep && (
        <CandidateStepper
          images={model.segment.images}
          bId={state.bId}
          position={compare.position}
          onStep={compare.step}
          compact
        />
      )}
      {state.mode === "toggle" && (
        <Button
          type="button"
          className={cn(reviewControlSize, "min-w-0 flex-1")}
          aria-pressed={state.showB}
          data-testid="question-review-flip"
          aria-label={t("task:questionReviewFlip")}
          onClick={() => compare.dispatch({ type: "flip" })}
        >
          <span className="truncate">{t("task:questionReviewFlipShort")}</span>
        </Button>
      )}
      {!stepping && (
        <div className="min-w-0 flex-1">
          <ModeControls compare={compare} wipe={model.wipe} />
        </div>
      )}
      {stepping && (
        <div className={cn(state.mode === "side" && "ml-auto")}>
          <ImagePickerSheetButton
            images={model.segment.images}
            urls={model.urls}
            aId={state.aId}
            bId={state.bId}
            onPick={(slot, id) => compare.dispatch({ type: "pick", slot, id })}
            onSwap={() => compare.dispatch({ type: "swap" })}
          />
        </div>
      )}
    </ReviewBottomBar>
  );
}

function CanvasArea({ model }: { model: CompareStageModel }) {
  const side = model.effective.mode === "side";
  return (
    <div
      ref={model.areaRef}
      className={cn("flex min-h-0 flex-1 gap-2", model.layout.phone ? "flex-col" : "flex-row")}
    >
      {side ? (
        <>
          <Pane model={model} pane="a" image={model.a} />
          <Pane model={model} pane="b" image={model.hold.holding ? model.a : model.b} />
        </>
      ) : (
        <Pane model={model} pane="both" />
      )}
    </div>
  );
}

function Pane({
  model,
  pane,
  image,
}: {
  model: CompareStageModel;
  pane: "a" | "b" | "both";
  image?: LoadedReviewImage;
}) {
  const both = pane === "both";
  const { viewport } = model;
  return (
    <ReviewCanvas
      stageRef={pane === "b" ? undefined : model.stageRef}
      view={viewport.view}
      onStageKey={model.onStageKey}
      onStageKeyUp={model.onStageKeyUp}
      pointerHandlers={both ? model.pointerHandlers : combinePane(model)}
      className={cn(both && model.marking && "cursor-crosshair ring-2 ring-primary ring-inset")}
      testId={pane === "b" ? "question-review-stage-b" : undefined}
      onDoubleClick={viewport.atFit ? viewport.actual : viewport.fit}
      overlay={<PaneOverlay model={model} pane={pane} />}
    >
      {both ? (
        <>
          <CompareLayers
            state={model.effective}
            a={model.a}
            b={model.b}
            view={viewport.view}
            stageWidth={viewport.stage.width}
            diff={model.diff}
            onSize={model.images.setSize}
            wipeAt={model.pinned ? 0.5 : undefined}
            outline={model.changes.outline}
          />
          {model.effective.mode === "toggle" && model.regions.overlay}
        </>
      ) : (
        image && (
          <ReviewImage
            key={image.url}
            src={image.url}
            alt={image.alt}
            scale={viewport.view.scale}
            onSize={(size) => model.images.setSize(image.id, size)}
          />
        )
      )}
    </ReviewCanvas>
  );
}

function combinePane(model: CompareStageModel) {
  const view = model.viewport.pointerHandlers;
  const hold = model.hold.pointer;
  return {
    onPointerDown: (event: React.PointerEvent<HTMLElement>) => {
      hold.onPointerDown(event);
      view.onPointerDown(event);
    },
    onPointerMove: (event: React.PointerEvent<HTMLElement>) => {
      hold.onPointerMove(event);
      view.onPointerMove(event);
    },
    onPointerUp: (event: React.PointerEvent<HTMLElement>) => {
      hold.onPointerUp(event);
      view.onPointerUp(event);
    },
    onPointerCancel: (event: React.PointerEvent<HTMLElement>) => {
      hold.onPointerCancel(event);
      view.onPointerCancel(event);
    },
  };
}

function PaneOverlay({ model, pane }: { model: CompareStageModel; pane: "a" | "b" | "both" }) {
  const primary = pane !== "b";
  return (
    <>
      {pane === "both" && <WipeOverlay model={model} />}
      {primary && model.regions.hint}
      {model.pending !== null && <ImageState state={model.pending} onRetry={model.images.retry} />}
      {primary && <ViewOverlays model={model} />}
    </>
  );
}

function WipeOverlay({ model }: { model: CompareStageModel }) {
  if (model.effective.mode !== "wipe" || !model.a || !model.b) return null;
  return model.pinned ? (
    <PinnedWipeLine value={model.wipe.value} onChange={model.wipe.onChange} />
  ) : (
    <WipeHandle value={model.wipe.value} onChange={model.wipe.onChange} />
  );
}

/** Zoom, Mark area, and status on the image: a chip on phones, a cluster on desktop. */
function ViewOverlays({ model }: { model: CompareStageModel }) {
  const { viewport } = model;
  const toggleZoom = viewport.atFit ? viewport.actual : viewport.fit;
  return (
    <>
      {model.layout.phone ? (
        <ZoomChip scale={viewport.view.scale} onToggle={toggleZoom} />
      ) : (
        <>
          <ZoomCluster
            scale={viewport.view.scale}
            onZoom={viewport.zoomStep}
            onFit={viewport.fit}
            onScale={viewport.zoomTo}
            changes={
              model.a && model.b
                ? { status: model.changes.status, onClick: model.changes.trigger }
                : undefined
            }
          />
          <StageOverlayControl>{model.regions.button}</StageOverlayControl>
        </>
      )}
      <StatusChips>
        <ReviewCompareStatus
          state={model.effective}
          a={model.aRef}
          b={model.bRef}
          sizes={model.images.sizes}
          diff={model.diff}
          chips
        />
      </StatusChips>
    </>
  );
}

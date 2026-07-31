"use client";

import { useCallback, useEffect, useRef, useState, type RefObject } from "react";
import type { KeyboardShortcut } from "@/lib/keyboard/constants";
import {
  hasHoldModifier,
  isCommitReleaseEvent,
  isCycleShortcutEvent,
  isHeldCycleKeyEvent,
} from "./recent-task-switcher-keys";
import type { TaskReviewTarget } from "./task-pr-open";

type UseTaskReviewShortcutOptions = {
  targets: TaskReviewTarget[];
  shortcut: KeyboardShortcut;
  onNoTargets: () => void;
  onOpenTarget: (target: TaskReviewTarget) => void;
};

function clampIndex(index: number, count: number): number {
  if (count === 0) return -1;
  return Math.min(Math.max(index, 0), count - 1);
}

function useLatestRef<T>(value: T) {
  const ref = useRef(value);
  ref.current = value;
  return ref;
}

type ReviewShortcutLifecycleOptions = {
  activeShortcutRef: RefObject<KeyboardShortcut | null>;
  cancelledRef: RefObject<boolean>;
  commitOnReleaseRef: RefObject<boolean>;
  openRef: RefObject<boolean>;
  advanceSelection: () => void;
  cancelPicker: () => void;
  openSelectedTarget: () => void;
};

function useReviewShortcutLifecycle({
  activeShortcutRef,
  cancelledRef,
  commitOnReleaseRef,
  openRef,
  advanceSelection,
  cancelPicker,
  openSelectedTarget,
}: ReviewShortcutLifecycleOptions) {
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && openRef.current) {
        event.preventDefault();
        event.stopPropagation();
        cancelPicker();
        return;
      }
      if (event.defaultPrevented || !openRef.current) return;
      const activeShortcut = activeShortcutRef.current;
      if (!activeShortcut || !isHeldCycleKeyEvent(event, activeShortcut)) return;
      event.preventDefault();
      event.stopPropagation();
      advanceSelection();
    };
    const handleKeyUp = (event: KeyboardEvent) => {
      if (!openRef.current || !commitOnReleaseRef.current) return;
      const activeShortcut = activeShortcutRef.current;
      if (!activeShortcut || !isCommitReleaseEvent(event, activeShortcut)) return;

      event.preventDefault();
      event.stopPropagation();
      if (cancelledRef.current) {
        cancelPicker();
        return;
      }
      openSelectedTarget();
    };
    const handleBlur = () => {
      if (openRef.current) cancelPicker();
    };
    const handleVisibilityChange = () => {
      if (document.visibilityState === "hidden" && openRef.current) cancelPicker();
    };

    window.addEventListener("keydown", handleKeyDown, true);
    window.addEventListener("keyup", handleKeyUp, true);
    window.addEventListener("blur", handleBlur);
    document.addEventListener("visibilitychange", handleVisibilityChange);
    return () => {
      window.removeEventListener("keydown", handleKeyDown, true);
      window.removeEventListener("keyup", handleKeyUp, true);
      window.removeEventListener("blur", handleBlur);
      document.removeEventListener("visibilitychange", handleVisibilityChange);
    };
  }, [
    activeShortcutRef,
    advanceSelection,
    cancelledRef,
    cancelPicker,
    commitOnReleaseRef,
    openRef,
    openSelectedTarget,
  ]);
}

function useReviewSelection(
  targets: TaskReviewTarget[],
  targetsRef: RefObject<TaskReviewTarget[]>,
) {
  const [selectedIndex, setSelectedIndexState] = useState(0);
  const selectedIndexRef = useRef(0);

  useEffect(() => {
    const nextIndex = clampIndex(selectedIndexRef.current, targets.length);
    if (nextIndex === selectedIndexRef.current) return;
    selectedIndexRef.current = nextIndex;
    setSelectedIndexState(nextIndex);
  }, [targets]);

  const setSelectedIndex = useCallback(
    (index: number) => {
      const nextIndex = clampIndex(index, targetsRef.current.length);
      selectedIndexRef.current = nextIndex;
      setSelectedIndexState(nextIndex);
    },
    [targetsRef],
  );

  return { selectedIndex, selectedIndexRef, setSelectedIndex };
}

type ReviewTargetActivationOptions = {
  targetsRef: RefObject<TaskReviewTarget[]>;
  selectedIndexRef: RefObject<number>;
  onOpenTargetRef: RefObject<(target: TaskReviewTarget) => void>;
  closePicker: () => void;
};

function useReviewTargetActivation({
  targetsRef,
  selectedIndexRef,
  onOpenTargetRef,
  closePicker,
}: ReviewTargetActivationOptions) {
  const openTargetAtIndex = useCallback(
    (index: number) => {
      const targets = targetsRef.current;
      const target = targets[clampIndex(index, targets.length)];
      closePicker();
      if (target) onOpenTargetRef.current(target);
    },
    [closePicker, onOpenTargetRef, targetsRef],
  );

  const openSelectedTarget = useCallback(
    () => openTargetAtIndex(selectedIndexRef.current),
    [openTargetAtIndex, selectedIndexRef],
  );

  return { openTargetAtIndex, openSelectedTarget };
}

export function useTaskReviewShortcut({
  targets,
  shortcut,
  onNoTargets,
  onOpenTarget,
}: UseTaskReviewShortcutOptions) {
  const [pickerOpen, setPickerOpenState] = useState(false);
  const activeShortcutRef = useRef<KeyboardShortcut | null>(null);
  const cancelledRef = useRef(false);
  const commitOnReleaseRef = useRef(false);
  const openRef = useRef(false);
  const onNoTargetsRef = useLatestRef(onNoTargets);
  const onOpenTargetRef = useLatestRef(onOpenTarget);
  const shortcutRef = useLatestRef(shortcut);
  const targetsRef = useLatestRef(targets);
  const { selectedIndex, selectedIndexRef, setSelectedIndex } = useReviewSelection(
    targets,
    targetsRef,
  );

  const closePicker = useCallback(() => {
    activeShortcutRef.current = null;
    commitOnReleaseRef.current = false;
    openRef.current = false;
    setPickerOpenState(false);
  }, []);

  const cancelPicker = useCallback(() => {
    cancelledRef.current = true;
    closePicker();
  }, [closePicker]);

  const setPickerOpen = useCallback(
    (nextOpen: boolean) => {
      if (!nextOpen) {
        cancelPicker();
        return;
      }
      cancelledRef.current = false;
      openRef.current = true;
      setPickerOpenState(true);
    },
    [cancelPicker],
  );

  const { openTargetAtIndex, openSelectedTarget } = useReviewTargetActivation({
    targetsRef,
    selectedIndexRef,
    onOpenTargetRef,
    closePicker,
  });

  const advanceSelection = useCallback(() => {
    const targetCount = targetsRef.current.length;
    if (targetCount < 2) return;
    setSelectedIndex((selectedIndexRef.current + 1) % targetCount);
  }, [setSelectedIndex, targetsRef]);

  const handleShortcut = useCallback(
    (event: KeyboardEvent) => {
      if (event.repeat) return;
      const reviewTargets = targetsRef.current;
      if (reviewTargets.length === 0) {
        onNoTargetsRef.current();
        return;
      }
      if (reviewTargets.length === 1) {
        onOpenTargetRef.current(reviewTargets[0]);
        return;
      }

      const activeShortcut = shortcutRef.current;
      if (!openRef.current) {
        if (!isCycleShortcutEvent(event, activeShortcut)) return;
        activeShortcutRef.current = activeShortcut;
        commitOnReleaseRef.current = hasHoldModifier(activeShortcut);
        cancelledRef.current = false;
        openRef.current = true;
        setSelectedIndex(0);
        setPickerOpenState(true);
        return;
      }
      const heldShortcut = activeShortcutRef.current ?? activeShortcut;
      if (isHeldCycleKeyEvent(event, heldShortcut)) advanceSelection();
    },
    [advanceSelection, onNoTargetsRef, onOpenTargetRef, setSelectedIndex, shortcutRef, targetsRef],
  );

  useReviewShortcutLifecycle({
    activeShortcutRef,
    cancelledRef,
    commitOnReleaseRef,
    openRef,
    advanceSelection,
    cancelPicker,
    openSelectedTarget,
  });

  return {
    pickerOpen,
    selectedIndex,
    setPickerOpen,
    setSelectedIndex,
    handleShortcut,
    openTargetAtIndex,
    openSelectedTarget,
  };
}

'use client';

import { useState, useEffect } from 'react';
import { SplitSide } from '@git-diff-view/react';
import { findRowsInRangeWithLineNumbers, groupByConsecutiveRowIndices } from './dom-utils';
import type { DragSelectionState } from './types';

interface DragSelectionOverlayProps {
  selection: DragSelectionState;
  wrapperRef: React.RefObject<HTMLDivElement | null>;
  viewMode: 'split' | 'unified';
}

interface OverlayRect {
  top: number;
  height: number;
  left: number | string;
  width: string;
}

export function DragSelectionOverlay({
  selection,
  wrapperRef,
  viewMode,
}: DragSelectionOverlayProps) {
  const [overlays, setOverlays] = useState<OverlayRect[]>([]);

  useEffect(() => {
    const updatePosition = () => {
      const wrapper = wrapperRef.current;
      if (!wrapper) return;

      const rangeStart = Math.min(selection.startLine, selection.endLine);
      const rangeEnd = Math.max(selection.startLine, selection.endLine);

      const rowsWithNumbers = findRowsInRangeWithLineNumbers(wrapper, viewMode, selection.side, rangeStart, rangeEnd);

      if (rowsWithNumbers.length === 0) {
        setOverlays([]);
        return;
      }

      const wrapperRect = wrapper.getBoundingClientRect();
      const left = viewMode === 'split' && selection.side === SplitSide.old ? 0 : viewMode === 'split' ? '50%' : 0;
      const width = viewMode === 'split' ? '50%' : '100%';

      // Group rows by consecutive row indices (split view) or all together (unified view)
      const groups = viewMode === 'split'
        ? groupByConsecutiveRowIndices(rowsWithNumbers)
        : [rowsWithNumbers.map(r => r.row)];

      const newOverlays: OverlayRect[] = groups.map((group) => {
        const firstRect = group[0].getBoundingClientRect();
        const lastRect = group[group.length - 1].getBoundingClientRect();

        return {
          top: firstRect.top - wrapperRect.top + wrapper.scrollTop,
          height: lastRect.bottom - firstRect.top,
          left,
          width,
        };
      });

      setOverlays(newOverlays);
    };

    updatePosition();
    const wrapper = wrapperRef.current;
    wrapper?.addEventListener('scroll', updatePosition);
    return () => wrapper?.removeEventListener('scroll', updatePosition);
  }, [selection, wrapperRef, viewMode]);

  if (overlays.length === 0) return null;

  return (
    <>
      {overlays.map((overlay, index) => (
        <div
          key={index}
          className="diff-multiline-selection-overlay"
          style={{
            display: 'block',
            position: 'absolute',
            top: overlay.top,
            height: overlay.height,
            left: overlay.left,
            width: overlay.width,
            backgroundColor: 'oklch(0.35 0.08 260 / 40%)',
            border: '2px solid oklch(0.65 0.15 260)',
            borderRadius: 2,
            pointerEvents: 'none',
            zIndex: 10,
          }}
        />
      ))}
    </>
  );
}

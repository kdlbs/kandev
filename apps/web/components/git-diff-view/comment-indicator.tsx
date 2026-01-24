'use client';

import { useState, useEffect } from 'react';
import { SplitSide } from '@git-diff-view/react';
import { findRowsInRangeWithLineNumbers, groupByConsecutiveRowIndices } from './dom-utils';
import type { DiffComment } from './types';

interface CommentRangeIndicatorProps {
  comment: DiffComment;
  wrapperRef: React.RefObject<HTMLDivElement | null>;
  viewMode: 'split' | 'unified';
}

interface IndicatorRect {
  top: number;
  height: number;
  left: number | string;
  width: string;
}

export function CommentRangeIndicator({
  comment,
  wrapperRef,
  viewMode,
}: CommentRangeIndicatorProps) {
  const [indicators, setIndicators] = useState<IndicatorRect[]>([]);

  useEffect(() => {
    const updatePosition = () => {
      const wrapper = wrapperRef.current;
      if (!wrapper) return;

      const rowsWithNumbers = findRowsInRangeWithLineNumbers(wrapper, viewMode, comment.side, comment.startLine, comment.endLine);

      if (rowsWithNumbers.length === 0) {
        setIndicators([]);
        return;
      }

      const wrapperRect = wrapper.getBoundingClientRect();
      const left = viewMode === 'split' && comment.side === SplitSide.old ? 0 : viewMode === 'split' ? '50%' : 0;
      const width = viewMode === 'split' ? '50%' : '100%';

      // Group rows by consecutive row indices (split view) or all together (unified view)
      const groups = viewMode === 'split'
        ? groupByConsecutiveRowIndices(rowsWithNumbers)
        : [rowsWithNumbers.map(r => r.row)];

      const newIndicators: IndicatorRect[] = groups.map((group) => {
        const firstRect = group[0].getBoundingClientRect();
        const lastRect = group[group.length - 1].getBoundingClientRect();

        return {
          top: firstRect.top - wrapperRect.top + wrapper.scrollTop,
          height: lastRect.bottom - firstRect.top,
          left,
          width,
        };
      });

      setIndicators(newIndicators);
    };

    updatePosition();
    const wrapper = wrapperRef.current;
    wrapper?.addEventListener('scroll', updatePosition);
    window.addEventListener('resize', updatePosition);
    return () => {
      wrapper?.removeEventListener('scroll', updatePosition);
      window.removeEventListener('resize', updatePosition);
    };
  }, [comment, wrapperRef, viewMode]);

  if (indicators.length === 0) return null;

  return (
    <>
      {indicators.map((indicator, index) => (
        <div
          key={index}
          style={{
            display: 'block',
            position: 'absolute',
            top: indicator.top,
            height: indicator.height,
            left: indicator.left,
            width: indicator.width,
            backgroundColor: 'oklch(0.75 0.12 80 / 15%)',
            borderRight: '3px solid oklch(0.7 0.15 80)',
            pointerEvents: 'none',
            zIndex: 5,
          }}
        />
      ))}
    </>
  );
}

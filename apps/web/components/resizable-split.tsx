'use client';

import { ReactNode, useCallback, useRef, useState } from 'react';
import { cn } from '@/lib/utils';

type Direction = 'horizontal' | 'vertical';

interface ResizableSplitProps {
  direction: Direction;
  initialSizes?: [number, number];
  minSizes?: [number, number];
  className?: string;
  handleClassName?: string;
  children: [ReactNode, ReactNode];
}

export function ResizableSplit({
  direction,
  initialSizes = [50, 50],
  minSizes = [20, 20],
  className,
  handleClassName,
  children,
}: ResizableSplitProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [sizes, setSizes] = useState<[number, number]>(initialSizes);
  const [isDragging, setIsDragging] = useState(false);

  const clampSizes = useCallback(
    (nextFirst: number) => {
      const minFirst = minSizes[0];
      const minSecond = minSizes[1];
      const clampedFirst = Math.min(100 - minSecond, Math.max(minFirst, nextFirst));
      return [clampedFirst, 100 - clampedFirst] as [number, number];
    },
    [minSizes]
  );

  const updateSizes = useCallback(
    (event: PointerEvent | React.PointerEvent) => {
      const rect = containerRef.current?.getBoundingClientRect();
      if (!rect) return;
      const total = direction === 'horizontal' ? rect.width : rect.height;
      if (!total) return;
      const offset =
        direction === 'horizontal' ? event.clientX - rect.left : event.clientY - rect.top;
      const nextFirst = (offset / total) * 100;
      setSizes(clampSizes(nextFirst));
    },
    [clampSizes, direction]
  );

  const handlePointerDown = useCallback(
    (event: React.PointerEvent<HTMLDivElement>) => {
      event.preventDefault();
      setIsDragging(true);
      updateSizes(event);

      const handleMove = (moveEvent: PointerEvent) => {
        updateSizes(moveEvent);
      };
      const handleUp = (upEvent: PointerEvent) => {
        updateSizes(upEvent);
        setIsDragging(false);
        window.removeEventListener('pointermove', handleMove);
        window.removeEventListener('pointerup', handleUp);
      };

      window.addEventListener('pointermove', handleMove);
      window.addEventListener('pointerup', handleUp);
    },
    [updateSizes]
  );

  const isHorizontal = direction === 'horizontal';

  return (
    <div
      ref={containerRef}
      className={cn(
        'flex min-h-0 min-w-0 h-full w-full',
        isHorizontal ? 'flex-row' : 'flex-col',
        className
      )}
    >
      <div
        className="min-h-0 min-w-0"
        style={{ flex: `0 0 ${sizes[0]}%` }}
      >
        {children[0]}
      </div>
      <div
        role="separator"
        aria-orientation={direction}
        onPointerDown={handlePointerDown}
        className={cn(
          isHorizontal
            ? 'w-2 cursor-col-resize border-l border-transparent'
            : 'h-2 cursor-row-resize border-t border-transparent',
          'bg-transparent hover:border-border/70',
          isDragging && 'border-border',
          handleClassName
        )}
      />
      <div
        className="min-h-0 min-w-0"
        style={{ flex: `1 1 ${sizes[1]}%` }}
      >
        {children[1]}
      </div>
    </div>
  );
}

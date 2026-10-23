export type SourcePosition = {
  selection?: { anchor: number; active: number };
  nearestLine: number;
};

export type SourcePositionHandle = {
  capture: () => SourcePosition | null;
  restore: (position: SourcePosition) => void;
};

export type SourcePositionProps = {
  onSourcePositionHandle?: (handle: SourcePositionHandle | null) => void;
  isActive?: boolean;
};

'use client';

// Re-export the existing Pierre/diffs-based DiffViewer as PierreDiffViewer
// The original implementation stays in components/diff/diff-viewer.tsx
export { DiffViewer as PierreDiffViewer, DiffViewInline as PierreInlineDiff } from '@/components/diff/diff-viewer';

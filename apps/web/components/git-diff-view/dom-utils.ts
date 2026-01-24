/**
 * DOM utilities for @git-diff-view/react library integration.
 *
 * This module uses the library's stable data attributes for DOM queries:
 * - data-line: line number on <tr> elements
 * - data-side: "old" | "new" on rows (split view only)
 * - data-state: "diff" | "plain" | "hunk" | "widget" | "extend"
 *
 * These data attributes are more stable than CSS class names across library versions.
 *
 * Fallback to class-based selectors is provided for unified view where
 * data-side is not present.
 */

import { SplitSide } from '@git-diff-view/react';

// Stable data attribute selectors
export const SELECTORS = {
  // Row with line number
  ROW_WITH_LINE: 'tr[data-line]',

  // Split view specific
  ROW_OLD_SIDE: 'tr[data-side="old"]',
  ROW_NEW_SIDE: 'tr[data-side="new"]',

  // Fallback class selectors for unified view
  CELL_LINE_NUM: 'td.diff-line-num',
  CELL_OLD_NUM: 'td.diff-line-old-num',
  CELL_NEW_NUM: 'td.diff-line-new-num',
  CELL_OLD_CONTENT: 'td.diff-line-old-content',
  CELL_NEW_CONTENT: 'td.diff-line-new-content',
} as const;

// Span indices in unified view line number cell
const UNIFIED_SPAN_OLD_INDEX = 0;
const UNIFIED_SPAN_NEW_INDEX = 2;

export interface LineInfo {
  lineNumber: number;
  side: SplitSide;
}

/**
 * Parse line number from data attribute or text content
 */
function parseLineNumber(value: string | null): number | null {
  if (!value) return null;
  const match = value.trim().match(/^[+-]?(\d+)$/);
  return match ? parseInt(match[1], 10) : null;
}

/**
 * Get line info from a row using data attributes (preferred) or class-based fallback
 */
function getLineInfoFromRow(
  row: Element,
  clickedElement: HTMLElement,
  viewMode: 'split' | 'unified'
): LineInfo | null {
  if (viewMode === 'split') {
    // Check if clicked on a placeholder cell (empty cell on one side) - return null
    const isOldPlaceholder = clickedElement.closest('td.diff-line-old-placeholder');
    const isNewPlaceholder = clickedElement.closest('td.diff-line-new-placeholder');
    if (isOldPlaceholder || isNewPlaceholder) {
      return null;
    }

    // Split view: determine which side was clicked and check if it has a line number
    const oldNumCell = row.querySelector(SELECTORS.CELL_OLD_NUM);
    const newNumCell = row.querySelector(SELECTORS.CELL_NEW_NUM);
    const oldContent = row.querySelector(SELECTORS.CELL_OLD_CONTENT);
    const newContent = row.querySelector(SELECTORS.CELL_NEW_CONTENT);

    // Check if clicked on old side
    if (oldNumCell && (oldNumCell.contains(clickedElement) || oldContent?.contains(clickedElement))) {
      const num = parseLineNumber(oldNumCell.textContent);
      // Only return if there's actually a line number (not an empty cell)
      if (num !== null) return { lineNumber: num, side: SplitSide.old };
      // Empty cell on old side - return null to skip
      return null;
    }

    // Check if clicked on new side
    if (newNumCell && (newNumCell.contains(clickedElement) || newContent?.contains(clickedElement))) {
      const num = parseLineNumber(newNumCell.textContent);
      // Only return if there's actually a line number (not an empty cell)
      if (num !== null) return { lineNumber: num, side: SplitSide.new };
      // Empty cell on new side - return null to skip
      return null;
    }

    // Fallback: try data attributes
    const dataLine = row.getAttribute('data-line');
    const dataSide = row.getAttribute('data-side');
    const lineNumber = parseLineNumber(dataLine);
    if (lineNumber !== null && dataSide) {
      return {
        lineNumber,
        side: dataSide === 'old' ? SplitSide.old : SplitSide.new,
      };
    }
  } else {
    // Unified view: no data-side, need to parse line number cell
    const lineNumCell = row.querySelector(SELECTORS.CELL_LINE_NUM);
    if (!lineNumCell) return null;

    const spans = lineNumCell.querySelectorAll('span');

    // Unified view span layout: [old number] [separator] [new number]
    let oldNum: number | null = null;
    let newNum: number | null = null;

    if (spans[UNIFIED_SPAN_OLD_INDEX]) {
      oldNum = parseLineNumber(spans[UNIFIED_SPAN_OLD_INDEX].textContent);
    }
    if (spans[UNIFIED_SPAN_NEW_INDEX]) {
      newNum = parseLineNumber(spans[UNIFIED_SPAN_NEW_INDEX].textContent);
    }

    // Prefer new side if available (additions show on new side)
    if (newNum !== null) {
      return { lineNumber: newNum, side: SplitSide.new };
    }
    if (oldNum !== null) {
      return { lineNumber: oldNum, side: SplitSide.old };
    }

    // Final fallback: parse all text
    const text = lineNumCell.textContent?.trim() || '';
    const numbers = text.match(/\d+/g);
    if (numbers && numbers.length > 0) {
      const num = parseInt(numbers[numbers.length - 1], 10);
      return {
        lineNumber: num,
        side: numbers.length > 1 ? SplitSide.new : SplitSide.old,
      };
    }
  }

  return null;
}

/**
 * Extract line number and side from a DOM element within the diff viewer
 */
export function getLineInfoFromElement(
  element: HTMLElement,
  wrapper: HTMLElement | null,
  viewMode: 'split' | 'unified'
): LineInfo | null {
  let current: HTMLElement | null = element;

  while (current && current !== wrapper) {
    const row = current.closest('tr');
    if (row) {
      const info = getLineInfoFromRow(row, element, viewMode);
      if (info) return info;
    }
    current = current.parentElement;
  }

  return null;
}

export interface RowWithLineNumber {
  row: HTMLElement;
  lineNumber: number;
  rowIndex: number; // Index in DOM for detecting visual gaps
}

/**
 * Find all rows in a line number range for overlay positioning
 */
export function findRowsInRange(
  wrapper: HTMLElement,
  viewMode: 'split' | 'unified',
  side: SplitSide,
  startLine: number,
  endLine: number
): HTMLElement[] {
  return findRowsInRangeWithLineNumbers(wrapper, viewMode, side, startLine, endLine).map(r => r.row);
}

/**
 * Find all rows in a line number range, returning both row and line number
 * This allows grouping by consecutive line numbers for split view
 */
export function findRowsInRangeWithLineNumbers(
  wrapper: HTMLElement,
  viewMode: 'split' | 'unified',
  side: SplitSide,
  startLine: number,
  endLine: number
): RowWithLineNumber[] {
  const items: RowWithLineNumber[] = [];

  if (viewMode === 'split') {
    // Get all rows to track row indices
    const allRows = Array.from(wrapper.querySelectorAll('tr'));
    const rowIndexMap = new Map<HTMLElement, number>();
    allRows.forEach((row, idx) => rowIndexMap.set(row as HTMLElement, idx));

    // Split view: use class-based selectors to get line numbers from cells
    const sideClass = side === SplitSide.old ? SELECTORS.CELL_OLD_NUM : SELECTORS.CELL_NEW_NUM;
    const lineNumCells = wrapper.querySelectorAll(sideClass);

    lineNumCells.forEach((cell) => {
      const num = parseLineNumber(cell.textContent);
      if (num !== null && num >= startLine && num <= endLine) {
        const row = cell.closest('tr') as HTMLElement;
        if (row) {
          const rowIndex = rowIndexMap.get(row) ?? -1;
          items.push({ row, lineNumber: num, rowIndex });
        }
      }
    });

    // Sort by row index to ensure DOM order
    items.sort((a, b) => a.rowIndex - b.rowIndex);
  } else {
    // Unified view: iterate through all line rows
    const allRows = Array.from(wrapper.querySelectorAll('tr'));
    const rowIndexMap = new Map<Element, number>();
    allRows.forEach((row, idx) => rowIndexMap.set(row, idx));

    const rows = wrapper.querySelectorAll(SELECTORS.ROW_WITH_LINE);

    rows.forEach((row) => {
      const lineNumCell = row.querySelector(SELECTORS.CELL_LINE_NUM);
      if (!lineNumCell) return;

      const spans = lineNumCell.querySelectorAll('span');
      let targetNum: number | null = null;

      // Check the appropriate span based on side - NO fallback to avoid matching wrong side
      const spanIndex = side === SplitSide.old ? UNIFIED_SPAN_OLD_INDEX : UNIFIED_SPAN_NEW_INDEX;
      if (spans[spanIndex]) {
        targetNum = parseLineNumber(spans[spanIndex].textContent);
      }

      // Only use fallback if there are no spans (old library version)
      if (targetNum === null && spans.length === 0) {
        const text = lineNumCell.textContent?.trim() || '';
        const numbers = text.match(/\d+/g);
        if (numbers && numbers.length > 0) {
          const idx = side === SplitSide.old ? 0 : numbers.length - 1;
          targetNum = parseInt(numbers[idx], 10);
        }
      }

      if (targetNum !== null && targetNum >= startLine && targetNum <= endLine) {
        const rowIndex = rowIndexMap.get(row) ?? -1;
        items.push({ row: row as HTMLElement, lineNumber: targetNum, rowIndex });
      }
    });

    // Sort by row index
    items.sort((a, b) => a.rowIndex - b.rowIndex);
  }

  // Remove duplicates based on row
  const seen = new Set<HTMLElement>();
  return items.filter(item => {
    if (seen.has(item.row)) return false;
    seen.add(item.row);
    return true;
  });
}

/**
 * Find a specific row by line number for widget positioning
 */
export function findRowByLineNumber(
  wrapper: HTMLElement,
  viewMode: 'split' | 'unified',
  side: SplitSide,
  lineNumber: number
): HTMLElement | null {
  const rows = findRowsInRange(wrapper, viewMode, side, lineNumber, lineNumber);
  return rows[0] || null;
}

/**
 * Group rows into contiguous ranges based on consecutive ROW INDICES.
 * This detects visual gaps caused by rows with empty cells on the selected side.
 * Used for split view to show separate overlays for non-adjacent rows.
 */
export function groupByConsecutiveRowIndices(items: RowWithLineNumber[]): HTMLElement[][] {
  if (items.length === 0) return [];

  const groups: HTMLElement[][] = [];
  let currentGroup: HTMLElement[] = [items[0].row];
  let lastRowIndex = items[0].rowIndex;

  for (let i = 1; i < items.length; i++) {
    const { row, rowIndex } = items[i];

    if (rowIndex === lastRowIndex + 1) {
      currentGroup.push(row);
    } else {
      groups.push(currentGroup);
      currentGroup = [row];
    }
    lastRowIndex = rowIndex;
  }

  groups.push(currentGroup);
  return groups;
}

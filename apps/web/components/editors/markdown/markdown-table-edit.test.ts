import { describe, expect, it } from "vitest";
import {
  appendMarkdownTableColumn,
  appendMarkdownTableRow,
  insertMarkdownTableColumn,
  insertMarkdownTableRow,
} from "./markdown-table-edit";

const insertRow = insertMarkdownTableRow;
const insertColumn = insertMarkdownTableColumn;

describe("appendMarkdownTableRow", () => {
  it("appends a row before trailing block separators and preserves CRLF", () => {
    const source = "| Variable | Default |\r\n| --- | --- |\r\n| HOME | /tmp |\r\n\r\n";

    const result = appendMarkdownTableRow(source, 2);

    expect(result.source).toBe(
      "| Variable | Default |\r\n| --- | --- |\r\n| HOME | /tmp |\r\n|  |  |\r\n\r\n",
    );
    expect(result.source[result.selectionOffset]).toBe(" ");
  });

  it("matches a table without outer pipes", () => {
    const source = "Variable | Default\n--- | ---\nHOME | /tmp\n";

    expect(appendMarkdownTableRow(source, 2).source).toBe(
      "Variable | Default\n--- | ---\nHOME | /tmp\n | \n",
    );
  });
});

describe("appendMarkdownTableColumn", () => {
  it("appends one cell to every row without changing existing cell bytes", () => {
    const source = "| Variable | Default |\n| :--- | ---: |\n| `A\\|B` | value |\n\n";

    const result = appendMarkdownTableColumn(source);

    expect(result.source).toBe(
      "| Variable | Default |  |\n| :--- | ---: | --- |\n| `A\\|B` | value |  |\n\n",
    );
    expect(result.source[result.selectionOffset]).toBe(" ");
  });

  it("preserves a table without outer pipes or a final newline", () => {
    const source = "Variable | Default\n--- | ---\nHOME | /tmp";

    expect(appendMarkdownTableColumn(source).source).toBe(
      "Variable | Default | \n--- | --- | ---\nHOME | /tmp | ",
    );
  });
});

describe("positional Markdown table edits", () => {
  it("inserts a body row directly below the header and keeps the delimiter adjacent", () => {
    const source = "| Name | Value |\r\n| :--- | ---: |\r\n| A\\|B | one |\r\n| C | two |\r\n\r\n";

    const result = insertRow(source, 0);

    expect(result.source).toBe(
      "| Name | Value |\r\n| :--- | ---: |\r\n|  |  |\r\n| A\\|B | one |\r\n| C | two |\r\n\r\n",
    );
    expect(result.source[result.selectionOffset]).toBe(" ");
  });

  it("inserts a row after the selected visible body row instead of appending", () => {
    const source = "| Name | Value |\n| --- | --- |\n| A | one |\n| B | two |\n";

    expect(insertRow(source, 1).source).toBe(
      "| Name | Value |\n| --- | --- |\n| A | one |\n|  |  |\n| B | two |\n",
    );
  });

  it("inserts a column after the requested column without splitting escaped pipes", () => {
    const source = "Name | Value | Notes\n:--- | ---: | ---\nA\\|B | one | keep\n\n";

    const result = insertColumn(source, 0);

    expect(result.source).toBe(
      "Name |  | Value | Notes\n:--- | --- | ---: | ---\nA\\|B |  | one | keep\n\n",
    );
    expect(result.source[result.selectionOffset]).toBe(" ");
  });

  it("inserts a column in the middle while preserving alignment markers and CRLF", () => {
    const source = "| Name | Value | Notes |\r\n| :--- | ---: | --- |\r\n| A | one | keep |\r\n";

    expect(insertColumn(source, 1).source).toBe(
      "| Name | Value |  | Notes |\r\n| :--- | ---: | --- | --- |\r\n| A | one |  | keep |\r\n",
    );
  });

  it("returns the source unchanged for an invalid visible row or column", () => {
    const source = "| Name | Value |\n| --- | --- |\n| A | one |\n";

    expect(insertRow(source, 2)).toEqual({ source, selectionOffset: 0 });
    expect(insertColumn(source, 2)).toEqual({ source, selectionOffset: 0 });
  });
});

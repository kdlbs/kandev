import { describe, expect, it } from "vitest";
import { appendMarkdownTableColumn, appendMarkdownTableRow } from "./markdown-table-edit";

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

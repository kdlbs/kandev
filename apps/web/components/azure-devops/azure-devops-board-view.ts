import type { AzureDevOpsBoard, AzureDevOpsBoardWorkItem } from "@/lib/types/azure-devops";

export function groupAzureDevOpsBoardItems(
  board: AzureDevOpsBoard,
  items: AzureDevOpsBoardWorkItem[],
): Map<string, AzureDevOpsBoardWorkItem[]> {
  const groups = new Map(
    board.columns.map((column) => [column.id, [] as AzureDevOpsBoardWorkItem[]]),
  );
  for (const item of items) {
    const group = groups.get(item.columnId);
    if (group) group.push(item);
  }
  return groups;
}

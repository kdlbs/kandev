export interface ExportFile {
  path: string;
  content: string;
}

export interface FileTreeNode {
  name: string;
  path: string;
  isDir: boolean;
  children: FileTreeNode[];
  content?: string;
}

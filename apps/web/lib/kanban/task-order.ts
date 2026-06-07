type CreatedTask = {
  createdAt?: string;
};

export function compareTasksByCreatedDesc(a: CreatedTask, b: CreatedTask): number {
  return (b.createdAt ?? "").localeCompare(a.createdAt ?? "");
}

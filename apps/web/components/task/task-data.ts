export const CHANGED_FILES = [
  { path: 'apps/web/components/kanban-board.tsx', status: 'M', plus: 12, minus: 4 },
  { path: 'apps/web/components/kanban-card.tsx', status: 'M', plus: 3, minus: 2 },
  { path: 'apps/web/components/task-create-dialog.tsx', status: 'A', plus: 55, minus: 0 },
  { path: 'apps/web/components/kanban-column.tsx', status: 'D', plus: 0, minus: 18 },
];

export const DIFF_SAMPLES: Record<string, { diff: string; newContent: string }> = {
  'apps/web/components/kanban-board.tsx': {
    diff: [
      'diff --git a/apps/web/components/kanban-board.tsx b/apps/web/components/kanban-board.tsx',
      'index 7b45ad2..9c0f2ee 100644',
      '--- a/apps/web/components/kanban-board.tsx',
      '+++ b/apps/web/components/kanban-board.tsx',
      '@@ -14,6 +14,7 @@ export function KanbanBoard() {',
      '   const columns = useMemo(() => getColumns(view), [view]);',
      '   const tasks = useMemo(() => getTasks(), []);',
      '+  const hasAlerts = tasks.some((task) => task.priority === "high");',
      '   return (',
      '     <div className="kanban-board">',
      '       <BoardHeader />',
    ].join('\n'),
    newContent: [
      'export function KanbanBoard() {',
      '  const columns = useMemo(() => getColumns(view), [view]);',
      '  const tasks = useMemo(() => getTasks(), []);',
      '  const hasAlerts = tasks.some((task) => task.priority === "high");',
      '  return (',
      '    <div className="kanban-board">',
      '      <BoardHeader />',
      '    </div>',
      '  );',
      '}',
    ].join('\n'),
  },
  'apps/web/components/kanban-card.tsx': {
    diff: [
      'diff --git a/apps/web/components/kanban-card.tsx b/apps/web/components/kanban-card.tsx',
      'index a14d022..a0cc12f 100644',
      '--- a/apps/web/components/kanban-card.tsx',
      '+++ b/apps/web/components/kanban-card.tsx',
      '@@ -8,7 +8,8 @@ export function KanbanCard({ task }: KanbanCardProps) {',
      '   return (',
      '     <Card className="kanban-card">',
      '-      <h4 className="title">{task.title}</h4>',
      '+      <h4 className="title">{task.title}</h4>',
      '+      <span className="tag">{task.assignee}</span>',
      '       <p className="summary">{task.summary}</p>',
      '     </Card>',
      '   );',
    ].join('\n'),
    newContent: [
      'export function KanbanCard({ task }: KanbanCardProps) {',
      '  return (',
      '    <Card className="kanban-card">',
      '      <h4 className="title">{task.title}</h4>',
      '      <span className="tag">{task.assignee}</span>',
      '      <p className="summary">{task.summary}</p>',
      '    </Card>',
      '  );',
      '}',
    ].join('\n'),
  },
  'apps/web/components/task-create-dialog.tsx': {
    diff: [
      'diff --git a/apps/web/components/task-create-dialog.tsx b/apps/web/components/task-create-dialog.tsx',
      'new file mode 100644',
      'index 0000000..6c1a1f0',
      '--- /dev/null',
      '+++ b/apps/web/components/task-create-dialog.tsx',
      '@@ -0,0 +1,6 @@',
      '+export function TaskCreateDialog() {',
      '+  return (',
      '+    <Dialog>',
      '+      <DialogContent>Create task</DialogContent>',
      '+    </Dialog>',
      '+  );',
      '+}',
    ].join('\n'),
    newContent: [
      'export function TaskCreateDialog() {',
      '  return (',
      '    <Dialog>',
      '      <DialogContent>Create task</DialogContent>',
      '    </Dialog>',
      '  );',
      '}',
    ].join('\n'),
  },
  'apps/web/components/kanban-column.tsx': {
    diff: [
      'diff --git a/apps/web/components/kanban-column.tsx b/apps/web/components/kanban-column.tsx',
      'deleted file mode 100644',
      'index 9a9b7aa..0000000',
      '--- a/apps/web/components/kanban-column.tsx',
      '+++ /dev/null',
      '@@ -1,5 +0,0 @@',
      '-export function KanbanColumn() {',
      '-  return (',
      '-    <section className="kanban-column">Column</section>',
      '-  );',
      '-}',
    ].join('\n'),
    newContent: '',
  },
};

export const COMMANDS = [
  { id: 'dev', label: 'npm run dev' },
  { id: 'lint', label: 'npm run lint' },
  { id: 'test', label: 'npm run test' },
];

export const FILE_TREE = [
  [
    'app',
    ['api', ['hello', ['route.ts']], 'page.tsx', 'layout.tsx', ['blog', ['page.tsx']]],
  ],
  ['components', ['ui', 'button.tsx', 'card.tsx'], 'header.tsx', 'footer.tsx'],
  ['lib', ['util.ts']],
  ['public', 'favicon.ico', 'vercel.svg'],
  '.eslintrc.json',
  '.gitignore',
  'next.config.js',
  'tailwind.config.js',
  'package.json',
  'README.md',
];

export function buildDiffData(filePath: string) {
  const sample = DIFF_SAMPLES[filePath];
  if (!sample) {
    return {
      hunks: [
        [
          `diff --git a/${filePath} b/${filePath}`,
          'index 0000000..0000000 100644',
          `--- a/${filePath}`,
          `+++ b/${filePath}`,
          '@@ -1,1 +1,1 @@',
          '-',
          '+',
        ].join('\n'),
      ],
      oldFile: { fileName: filePath, fileLang: 'ts' },
      newFile: { fileName: filePath, fileLang: 'ts' },
    };
  }

  return {
    hunks: [sample.diff],
    oldFile: { fileName: filePath, fileLang: 'ts' },
    newFile: { fileName: filePath, fileLang: 'ts' },
  };
}

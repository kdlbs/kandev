// Read-only, heuristic inventory. This is not an automatic test selector. Run from the repository root.
const fs = require("node:fs"),
  path = require("node:path"),
  cp = require("node:child_process"),
  { createRequire } = require("node:module");
const root = process.cwd(),
  web = path.join(root, "apps/web"),
  req = createRequire(path.join(web, "package.json")),
  ts = req("typescript");
const all = cp
  .execFileSync("rg", ["--files", "apps/web", "apps/packages"], {
    encoding: "utf8",
  })
  .trim()
  .split("\n")
  .filter((f) => /\.[cm]?[jt]sx?$/.test(f));
const exists = new Set(all.map((f) => path.resolve(root, f)));
const graph = new Map();
function resolve(from, s) {
  let p;
  if (s.startsWith(".")) p = path.resolve(path.dirname(from), s);
  else if (s.startsWith("@/")) p = path.join(web, s.slice(2));
  else return null;
  for (const c of [
    p,
    ...[
      ".ts",
      ".tsx",
      ".js",
      ".mjs",
      ".cjs",
      ".mts",
      ".cts",
      "/index.ts",
      "/index.tsx",
    ].map((e) => p + e),
  ])
    if (exists.has(c)) return c;
  return null;
}
for (const f of exists) {
  const src = fs.readFileSync(f, "utf8"),
    ast = ts.createSourceFile(f, src, ts.ScriptTarget.Latest, true);
  const row = {
    file: path.relative(web, f),
    imports: [],
    deps: [],
    dom: false,
    locale: false,
    sourceRead: false,
    dynamic: false,
    tests: 0,
  };
  function visit(n) {
    if (ts.isImportDeclaration(n) || ts.isExportDeclaration(n)) {
      if (
        n.moduleSpecifier &&
        ts.isStringLiteral(n.moduleSpecifier) &&
        !n.importClause?.isTypeOnly &&
        !n.isTypeOnly
      ) {
        const s = n.moduleSpecifier.text;
        row.imports.push(s);
        const dep = resolve(f, s);
        if (dep) row.deps.push(dep);
      }
    }
    if (ts.isCallExpression(n)) {
      const callee = n.expression.getText(ast);
      if (
        /(^|\.)(changeLanguage|activateLocale|loadAllLocalesForTests|loadLocale|addResourceBundle|removeResourceBundle)$/.test(
          callee,
        )
      )
        row.locale = true;
      if (/(^|\.)(readFileSync|readFile)$/.test(callee)) row.sourceRead = true;
      if (/(^|\.)(test|it)(\.(each|skip|only))?$/.test(callee)) row.tests++;
      if (n.expression.kind === ts.SyntaxKind.ImportKeyword) {
        if (!n.arguments[0] || !ts.isStringLiteral(n.arguments[0]))
          row.dynamic = true;
        else {
          const dep = resolve(f, n.arguments[0].text);
          if (dep) row.deps.push(dep);
        }
      }
    }
    if (
      ts.isIdentifier(n) &&
      [
        "window",
        "document",
        "localStorage",
        "sessionStorage",
        "navigator",
        "HTMLElement",
        "WebSocket",
        "ResizeObserver",
        "MutationObserver",
      ].includes(n.text)
    )
      row.dom = true;
    ts.forEachChild(n, visit);
  }
  visit(ast);
  if (
    row.imports.some((s) =>
      /^(react|react-dom|@testing-library|happy-dom|@kandev\/ui)/.test(s),
    )
  )
    row.dom = true;
  graph.set(f, row);
}
const tests = [...graph].filter(
  ([f]) =>
    /\.(test|spec)\.[cm]?[jt]sx?$/.test(f) &&
    f.startsWith(web + "/") &&
    !/\/e2e\/(fixtures|pages)\//.test(f) &&
    !(/\/e2e\//.test(f) && f.endsWith(".spec.ts")),
);
const rows = tests.map(([f, r]) => {
  const seen = new Set(),
    agg = { dom: false, locale: false, dynamic: false },
    external = new Set();
  function walk(k) {
    if (seen.has(k)) return;
    seen.add(k);
    const n = graph.get(k);
    if (!n) return;
    for (const s of n.imports)
      if (!s.startsWith(".") && !s.startsWith("@/")) external.add(s);
    agg.dom ||= n.dom;
    agg.dynamic ||= n.dynamic;
    // The shared i18n module defines activation functions; their mere presence does not establish a caller uses them.
    if (k !== path.join(web, "lib/i18n/index.ts")) agg.locale ||= n.locale;
    for (const d of n.deps) walk(d);
  }
  walk(f);
  return {
    path: r.file,
    directLocale: r.locale,
    directDom: r.dom,
    sourceRead: r.sourceRead,
    transitiveDom: agg.dom,
    transitiveLocale: agg.locale,
    unresolvedDynamic: agg.dynamic,
    localModules: seen.size,
    external: [...external].sort(),
    candidate:
      agg.locale || agg.dynamic
        ? "retain-full-or-review"
        : agg.dom
          ? "review-english-browser"
          : "review-node",
  };
});
fs.writeFileSync(
  process.argv[2] || "/tmp/kandev-frontend-audit.json",
  JSON.stringify(rows, null, 2),
);
const counts = {};
for (const r of rows) counts[r.candidate] = (counts[r.candidate] || 0) + 1;
console.log(
  JSON.stringify(
    {
      files: rows.length,
      counts,
      directLocale: rows.filter((x) => x.directLocale).length,
      sourceRead: rows.filter((x) => x.sourceRead).length,
    },
    null,
    2,
  ),
);
for (const candidate of ["review-node", "review-english-browser"])
  console.log(
    candidate,
    rows
      .filter((x) => x.candidate === candidate && x.localModules <= 4)
      .slice(0, 16)
      .map((x) => x.path),
  );

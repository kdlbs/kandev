const fs = require("node:fs");

const report = JSON.parse(fs.readFileSync(process.argv[2], "utf8"));
const tests = [];
function collect(suite) {
  for (const spec of suite.specs ?? []) tests.push(...(spec.tests ?? []));
  for (const child of suite.suites ?? []) collect(child);
}
for (const suite of report.suites ?? []) collect(suite);
const stats = report.stats ?? {};
const executed =
  tests.length === 5 &&
  tests.every(
    (test) =>
      test.expectedStatus === "passed" &&
      test.results?.length === 1 &&
      test.results[0].status === "passed" &&
      test.results[0].retry === 0,
  );
if (
  !executed ||
  stats.expected !== 5 ||
  stats.unexpected !== 0 ||
  stats.skipped !== 0 ||
  stats.flaky !== 0 ||
  report.errors?.length
) {
  throw new Error(
    "Kubernetes acceptance requires five executed passes with no retries, skips or setup errors",
  );
}
console.log(
  "Five Kubernetes acceptance scenarios passed without retries or skips",
);

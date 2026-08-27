# `--repeat-each` parity fixture

A real blob report from `npx playwright test --repeat-each=4 --retries=1` over a
single test that fails its first attempt on two of the four repetitions, plus
the `stats` block Playwright produced from that same blob.

It exists because `--repeat-each` is the flag used to verify de-flaking work, and
the summary used to be wrong for exactly that case. All four repetitions share
one project/file/title, so grouping on `key` collapsed them into a single test
whose only surviving attempt was retry `0` of the last repetition: the summary
reported `0 flaky of 1 executed` where Playwright reported `2 flaky of 4`.
Grouping on Playwright's test id, which is distinct per repetition, is what makes
the two agree.

Regenerate with the recipe in `../playwright-parity/README.md`, replacing the
spec with a single test that fails the first attempt of some repetitions, and
passing `--repeat-each=4`.

// Run from the repository root: node .agents/skills/pr-walkthrough/references/test_browser.cjs
// Uses the workspace Playwright installation and the standalone HTML renderer.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { pathToFileURL } = require('node:url');
const { createRequire } = require('node:module');
const { execFileSync } = require('node:child_process');
const workspaceRequire = createRequire(path.resolve(__dirname, '../../../../apps/web/package.json'));
const { chromium } = workspaceRequire('@playwright/test');

async function main() {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'walkthrough-browser-'));
  const browser = await chromium.launch({ headless: true });
  try {
    const data = JSON.parse(fs.readFileSync(path.join(__dirname, 'example.json'), 'utf8'));
    // Synthetic changes exercise every table without implying facts about a real PR.
    const rows = {
      ux: { surface: 'Settings > Models', before: 'Automatic fallback', after: 'Mismatch error' },
      plugins: { name: 'HostUI.selectModel()', change: 'changed', contract: 'Requires a model ID', compatibility: 'Breaking: pass the configured ID' },
      mcp: { name: 'select_model', context: 'Task MCP', change: 'added', contract: 'Selects a configured model', compatibility: 'Compatible addition' },
      database: { migration: '042 / task database', object: 'tasks.model_id', change: 'Adds nullable TEXT column', upgrade: 'Existing rows retain NULL. Rollback removes the column.' },
    };
    for (const [key, row] of Object.entries(rows)) {
      data.impact[key] = { status: 'changed', items: [{ ...row, file: 'src/model.ts' }] };
    }
    const input = path.join(dir, 'test.json');
    const output = path.join(dir, 'test.html');
    fs.writeFileSync(input, JSON.stringify(data));
    execFileSync('python3', [path.join(__dirname, 'build.py'), input, output]);
    for (const width of [390, 767, 768, 1023, 1024, 1440]) {
      const page = await browser.newPage({ viewport: { width, height: 900 } });
      await page.goto(pathToFileURL(output).href);
      await page.waitForFunction(() => getComputedStyle(document.querySelector('main')).maxWidth !== 'none');
      for (const dark of [true, false]) {
        await page.evaluate((value) => document.documentElement.classList.toggle('dark', value), dark);
        assert.equal(await page.locator('#impact table').count(), 5);
        assert.match(await page.locator('#impact').innerText(), /Breaking changes detected/);
        assert.match(await page.locator('#impact-mcp').innerText(), /net \+1/);
        assert.match(await page.locator('#impact-database').innerText(), /Existing rows retain NULL/);
        const geometry = await page.locator('#impact').evaluate((el) => ({
          fits: [...el.querySelectorAll('td')].every(cell => cell.getBoundingClientRect().right <= innerWidth),
          rowDisplay: getComputedStyle(el.querySelector('tr')).display,
          label: getComputedStyle(el.querySelector('td'), '::before').content,
          overflow: document.documentElement.scrollWidth > innerWidth,
        }));
        assert.equal(geometry.fits, true);
        assert.equal(geometry.overflow, false);
        assert.equal(geometry.rowDisplay, width < 768 ? 'block' : 'table-row');
        if (width < 768) {
          assert.match(geometry.label, /Affected users/);
          assert.ok((await page.locator('#impact-ux a').boundingBox()).height >= 44);
        }
      }
      // The shipped menu opens below the shell's desktop navigation breakpoint.
      const mobileLink = page.locator('#mobile-nav-impact');
      const desktopLink = page.locator('#nav-impact');
      if (await desktopLink.isVisible()) {
        await desktopLink.click();
      } else {
        await page.locator('#mobile-nav summary').click();
        await mobileLink.click();
      }
      assert.equal(new URL(page.url()).hash, '#impact');
      assert.ok(await page.locator('#impact-ux a').getAttribute('href').then(url => url.includes('/files#diff-')));
      await page.close();
      console.log(`PASS ${width}px: impact content, themes, layout, source links, navigation`);
    }
  } finally {
    await browser.close();
    fs.rmSync(dir, { recursive: true, force: true });
  }
}

main().catch(error => { console.error(error); process.exitCode = 1; });

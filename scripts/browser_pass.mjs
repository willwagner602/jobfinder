// Lane 2: render candidate URLs headlessly, extract careers evidence.
// Input: JSON queue {rows:[{source_id,name,urls:[...]}]}; Output: JSON results.
import { createRequire } from 'module';
import fs from 'fs';
const require = createRequire('/home/wbw/work/jobfinder/frontend/package.json');
const { chromium } = require('playwright');

const queuePath = process.argv[2];
const outPath = process.argv[3];
const queue = JSON.parse(fs.readFileSync(queuePath, 'utf8'));
const rows = queue.rows || [];
const out = [];

const CAREER_WORDS = /(careers|career|jobs|job-openings|open-positions|join-our-team|work-with-us|we-are-hiring|hiring|vacancies|opportunities)/i;

function evidenceOf(text, links, name) {
  const t = (text || '').toLowerCase();
  const nl = links.filter(l => CAREER_WORDS.test(l));
  const namePresent = name && t.includes(name.toLowerCase().split(/[ ,(]/)[0]);
  const careersWords = /(apply now|join our team|current openings|open roles|job openings|view all jobs|search jobs|we are hiring|careers)/.test(t);
  return { namePresent, careersWords, atsLinks: nl.length, candidates: nl.slice(0, 5) };
}

const browser = await chromium.launch({ headless: true });
const ctx = await browser.newContext({
  userAgent: 'Mozilla/5.0 (X11; Linux x86_64; rv:132.0) Gecko/20100101 Firefox/132.0',
  viewport: { width: 1280, height: 900 },
  locale: 'en-US',
});
let n = 0;
for (const row of rows) {
  const rec = { ...row, attempts: [] };
  for (const url of (row.urls || []).slice(0, 3)) {
    const page = await ctx.newPage();
    try {
      const resp = await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 25000 });
      await page.waitForTimeout(2500); // allow JS hydration
      const text = (await page.locator('body').innerText({ timeout: 8000 }).catch(() => '')) || '';
      const links = (await page.locator('a').evaluateAll(
        as => as.map(a => a.href).filter(Boolean)));
      const ev = evidenceOf(text, links, row.name);
      rec.attempts.push({
        url, finalUrl: page.url(), status: resp ? resp.status() : null,
        textLength: text.length, ...ev,
      });
      if (ev.namePresent && (ev.careersWords || ev.atsLinks)) break; // good enough
    } catch (e) {
      rec.attempts.push({ url, error: String(e).slice(0, 140) });
    } finally {
      await page.close();
    }
  }
  rec.confirmed = rec.attempts.some(a => a.namePresent && (a.careersWords || a.atsLinks) && a.status === 200);
  out.push(rec);
  n++;
  if (n % 3 === 0 || n === rows.length) {
    console.log(`rendered ${n}/${rows.length}`, flush());
    fs.writeFileSync(outPath, JSON.stringify(out, null, 1));
  }
}
fs.writeFileSync(outPath, JSON.stringify(out, null, 1));
await browser.close();
console.log(`done ${out.filter(r => r.confirmed).length}/${out.length} confirmed`);

function flush() { return ''; }

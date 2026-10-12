#!/usr/bin/env node
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const projectRoot = resolve(here, '../..');
const outputRoot = resolve(projectRoot, 'artifacts/ui-screenshots');
const matrix = JSON.parse(await readFile(resolve(here, 'screenshot-matrix.json'), 'utf8'));
const fixtures = JSON.parse(await readFile(resolve(here, 'fixtures.json'), 'utf8'));
const themes = ['light', 'dark'];
const viewports = [
  { name: 'desktop', width: 1440, height: 1000 },
  { name: 'mobile', width: 390, height: 844 },
];

function escapeHtml(value) {
  return String(value)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;');
}

function pageSummary(page, state) {
  if (page.page === 'dashboard') return `${fixtures.users[0].email} with ${fixtures.jobs.length} representative matches`;
  if (page.page === 'profile-onboarding') return fixtures.profile.resumeText;
  if (page.page.startsWith('job')) return fixtures.jobs.map((job) => `${job.title} at ${job.company}`).join('; ');
  if (state.includes('error')) return 'Synthetic validation or server error state';
  return 'Synthetic local-only preview data';
}

function svgFor({ page, state, theme, viewport }) {
  const dark = theme === 'dark';
  const bg = dark ? '#0f172a' : '#f8fafc';
  const card = dark ? '#1e293b' : '#ffffff';
  const fg = dark ? '#e2e8f0' : '#0f172a';
  const muted = dark ? '#94a3b8' : '#475569';
  const accent = dark ? '#38bdf8' : '#2563eb';
  const summary = pageSummary(page, state);
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${viewport.width}" height="${viewport.height}" viewBox="0 0 ${viewport.width} ${viewport.height}">
  <rect width="100%" height="100%" fill="${bg}"/>
  <rect x="32" y="32" width="${viewport.width - 64}" height="84" rx="18" fill="${card}" stroke="${accent}"/>
  <text x="56" y="82" font-family="Inter, Arial, sans-serif" font-size="28" font-weight="700" fill="${fg}">jobs.willwagner.me</text>
  <text x="56" y="104" font-family="Inter, Arial, sans-serif" font-size="14" fill="${muted}">${escapeHtml(theme)} theme · ${escapeHtml(viewport.name)} ${viewport.width}×${viewport.height}</text>
  <rect x="32" y="150" width="${viewport.width - 64}" height="${viewport.height - 190}" rx="24" fill="${card}"/>
  <text x="64" y="210" font-family="Inter, Arial, sans-serif" font-size="18" font-weight="700" fill="${accent}">${escapeHtml(page.path)}</text>
  <text x="64" y="265" font-family="Inter, Arial, sans-serif" font-size="42" font-weight="800" fill="${fg}">${escapeHtml(page.page)}</text>
  <text x="64" y="314" font-family="Inter, Arial, sans-serif" font-size="26" fill="${fg}">State: ${escapeHtml(state)}</text>
  <foreignObject x="64" y="350" width="${viewport.width - 128}" height="220">
    <div xmlns="http://www.w3.org/1999/xhtml" style="font: 20px Inter, Arial, sans-serif; color: ${muted}; line-height: 1.5;">
      ${escapeHtml(summary)}
    </div>
  </foreignObject>
  <text x="64" y="${viewport.height - 76}" font-family="Inter, Arial, sans-serif" font-size="16" fill="${muted}">Deterministic synthetic preview artifact. Not produced from production data or credentials.</text>
</svg>
`;
}

await mkdir(outputRoot, { recursive: true });
const manifest = [];
for (const page of matrix) {
  for (const state of page.states) {
    for (const theme of themes) {
      for (const viewport of viewports) {
        const file = `${page.page}--${state}--${theme}--${viewport.name}.svg`;
        const path = resolve(outputRoot, file);
        await writeFile(path, svgFor({ page, state, theme, viewport }), 'utf8');
        manifest.push({ file, page: page.page, path: page.path, state, theme, viewport });
      }
    }
  }
}
await writeFile(resolve(outputRoot, 'manifest.json'), `${JSON.stringify(manifest, null, 2)}\n`, 'utf8');
console.log(`Generated ${manifest.length} deterministic SVG screenshot placeholders in ${outputRoot}`);

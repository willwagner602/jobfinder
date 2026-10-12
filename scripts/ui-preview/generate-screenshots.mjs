#!/usr/bin/env node
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { createServer } from 'node:http';
import { createReadStream, existsSync } from 'node:fs';
import { extname, join, resolve } from 'node:path';
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import {
  apiFailures,
  defaultJobs,
  defaultProfile,
  installMockApi,
  makeBrowserApiState,
  setThemeBeforeNavigation,
} from './browser-fixtures.mjs';

const here = resolve(fileURLToPath(new URL('.', import.meta.url)));
const projectRoot = resolve(here, '../..');
const frontendRoot = resolve(projectRoot, 'frontend');
const outputRoot = resolve(projectRoot, 'artifacts/ui-screenshots');
const matrix = JSON.parse(await readFile(resolve(here, 'screenshot-matrix.json'), 'utf8'));
const themes = ['light', 'dark'];
const viewports = [
  { name: 'desktop', width: 1440, height: 1000 },
  { name: 'mobile', width: 390, height: 844 },
];
const requestedPort = process.env.UI_SCREENSHOT_PORT ? Number(process.env.UI_SCREENSHOT_PORT) : 0;
let baseURL = 'http://127.0.0.1:0';
const { chromium } = await import(resolve(frontendRoot, 'node_modules/playwright/index.mjs'));

const mime = new Map([
  ['.html', 'text/html; charset=utf-8'],
  ['.js', 'text/javascript; charset=utf-8'],
  ['.css', 'text/css; charset=utf-8'],
  ['.svg', 'image/svg+xml'],
  ['.png', 'image/png'],
  ['.ico', 'image/x-icon'],
]);

function chromiumExecutablePath() {
  if (process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH) return process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH;
  if (process.env.CHROMIUM_PATH) return process.env.CHROMIUM_PATH;
  if (existsSync('/snap/bin/chromium')) return '/snap/bin/chromium';
  return undefined;
}

async function run(command, args, options = {}) {
  await new Promise((resolvePromise, reject) => {
    const child = spawn(command, args, { stdio: 'inherit', ...options });
    child.on('exit', (code) => code === 0 ? resolvePromise() : reject(new Error(`${command} ${args.join(' ')} exited with ${code}`)));
    child.on('error', reject);
  });
}

async function ensureBuilt() {
  if (existsSync(resolve(frontendRoot, 'dist/index.html'))) return;
  await run('npm', ['run', 'build'], { cwd: frontendRoot });
}

function startStaticServer() {
  const dist = resolve(frontendRoot, 'dist');
  const server = createServer((request, response) => {
    const url = new URL(request.url || '/', baseURL);
    let pathname = decodeURIComponent(url.pathname);
    if (pathname === '/') pathname = '/index.html';
    const candidate = resolve(dist, `.${pathname}`);
    const file = candidate.startsWith(dist) && existsSync(candidate) ? candidate : resolve(dist, 'index.html');
    response.setHeader('Content-Type', mime.get(extname(file)) || 'application/octet-stream');
    createReadStream(file).pipe(response);
  });
  return new Promise((resolvePromise, reject) => {
    server.once('error', reject);
    server.listen(requestedPort, '127.0.0.1', () => {
      const address = server.address();
      if (!address || typeof address === 'string') return reject(new Error('Could not determine screenshot server address'));
      baseURL = `http://127.0.0.1:${address.port}`;
      resolvePromise(server);
    });
  });
}

function stateFor(pageName, state) {
  const base = {};
  if (['signup', 'verify', 'login'].includes(pageName)) base.authenticated = false;

  if (state === 'empty') return { ...base, jobs: [], profile: { ...defaultProfile, resumeText: '', experienceText: '', lastMatchRunMessage: '' } };
  if (state === 'no-summary') return { ...base, jobs: [{ ...defaultJobs[0], summary: '', canonicalUrl: '' }] };

  const fail = {};
  if (pageName === 'signup' && state === 'server-error') fail['/api/signup'] = apiFailures.signup;
  if (pageName === 'verify' && state === 'server-error') fail['/api/verify'] = apiFailures.verify;
  if (pageName === 'login' && state === 'server-error') fail['/api/login'] = apiFailures.login;
  if (pageName === 'dashboard' && state === 'request-failure') fail['/api/dashboard'] = apiFailures.dashboard;
  if (pageName === 'profile-onboarding' && state === 'request-failure') fail['/api/profile'] = apiFailures.profileLoad;
  if (pageName === 'job-matches' && state === 'request-failure') fail['/api/jobs'] = apiFailures.jobs;
  if (pageName === 'job-match-detail' && state === 'request-failure') fail['/api/jobs/101'] = apiFailures.jobDetail;
  return { ...base, fail };
}

async function driveState(page, row, state) {
  const releaseLoadingRoutes = [];
  if (state === 'loading') {
    await page.route('**/api/**', async (route) => {
      await new Promise((resolve) => releaseLoadingRoutes.push(resolve));
      await route.fulfill({ status: 200, contentType: 'application/json; charset=utf-8', body: '{}' }).catch(() => {});
    });
  }

  await page.goto(`${baseURL}${row.path}`);

  if (state === 'validation-error') {
    if (row.page === 'signup') await page.getByRole('button', { name: 'Create account and send code' }).click();
    if (row.page === 'verify') await page.getByRole('button', { name: 'Verify and continue' }).click();
    if (row.page === 'login') await page.getByRole('button', { name: 'Log in', exact: true }).click();
    if (row.page === 'profile-onboarding') {
      await page.getByLabel('Resume text').fill('');
      await page.getByLabel('Experience bullets').fill('');
      await page.getByRole('button', { name: 'Save and request prior-week match run' }).click();
    }
  }

  if (state === 'server-error') {
    if (row.page === 'signup') {
      await page.getByLabel('Email').fill('blocked@example.test');
      await page.getByRole('button', { name: 'Create account and send code' }).click();
    }
    if (row.page === 'verify') {
      await page.getByLabel('One-time code').fill('000000');
      await page.getByRole('button', { name: 'Verify and continue' }).click();
    }
    if (row.page === 'login') {
      await page.getByLabel('Email').fill('preview.user@example.test');
      await page.getByLabel('Password').fill('bad-password');
      await page.getByRole('button', { name: 'Log in', exact: true }).click();
    }
  }

  if (state === 'loading') {
    await page.waitForTimeout(300);
    for (const release of releaseLoadingRoutes) release();
  } else {
    await page.waitForLoadState('domcontentloaded').catch(() => {});
    await page.waitForTimeout(100);
  }
}

await ensureBuilt();
await mkdir(outputRoot, { recursive: true });
const server = await startStaticServer();
const browser = await chromium.launch({ executablePath: chromiumExecutablePath(), headless: true, args: ['--no-sandbox', '--disable-dev-shm-usage'] });
const manifest = [];

try {
  for (const row of matrix) {
    for (const state of row.states) {
      for (const theme of themes) {
        for (const viewport of viewports) {
          const context = await browser.newContext({ viewport, deviceScaleFactor: 1 });
          const page = await context.newPage();
          await setThemeBeforeNavigation(page, theme);
          await installMockApi(page, makeBrowserApiState(stateFor(row.page, state)));
          await driveState(page, row, state);
          const file = `${row.page}--${state}--${theme}--${viewport.name}.png`;
          const path = resolve(outputRoot, file);
          await page.screenshot({ path, fullPage: true });
          await context.close();
          manifest.push({ file, page: row.page, path: row.path, state, theme, viewport });
        }
      }
    }
  }
} finally {
  await browser.close();
  await new Promise((resolvePromise) => server.close(resolvePromise));
}

await writeFile(resolve(outputRoot, 'manifest.json'), `${JSON.stringify(manifest, null, 2)}\n`, 'utf8');
console.log(`Generated ${manifest.length} browser screenshots from the Vue app in ${outputRoot}`);

import { defineConfig, devices } from '@playwright/test';
import { existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';

const chromiumExecutablePath = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH
  || process.env.CHROMIUM_PATH
  || (existsSync('/snap/bin/chromium') ? '/snap/bin/chromium' : undefined);
const frontendRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');

export default defineConfig({
  testDir: '.',
  testMatch: /.*\.e2e\.ts/,
  fullyParallel: false,
  reporter: [['list']],
  use: {
    baseURL: 'http://127.0.0.1:4173',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    ...devices['Desktop Chrome'],
    launchOptions: chromiumExecutablePath ? { executablePath: chromiumExecutablePath } : {},
  },
  webServer: {
    command: 'npx vite preview --host 127.0.0.1 --port 4173',
    cwd: frontendRoot,
    url: 'http://127.0.0.1:4173',
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
});

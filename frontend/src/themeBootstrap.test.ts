import { describe, expect, it, vi } from 'vitest';

const bootstrap = `
(() => {
  const stored = localStorage.getItem('jobs-theme');
  const dark = stored ? stored === 'dark' : matchMedia('(prefers-color-scheme: dark)').matches;
  document.documentElement.classList.toggle('dark', dark);
})();
`;

function runBootstrap(stored: string | null, systemDark: boolean) {
  localStorage.clear();
  document.documentElement.className = '';
  if (stored) localStorage.setItem('jobs-theme', stored);
  vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({ matches: systemDark }));
  Function(bootstrap)();
  return document.documentElement.classList.contains('dark');
}

describe('theme bootstrap script', () => {
  it('uses the persisted jobs-theme override before system preference', () => {
    expect(runBootstrap('light', true)).toBe(false);
    expect(runBootstrap('dark', false)).toBe(true);
  });

  it('follows system preference when no override is stored', () => {
    expect(runBootstrap(null, true)).toBe(true);
    expect(runBootstrap(null, false)).toBe(false);
  });
});

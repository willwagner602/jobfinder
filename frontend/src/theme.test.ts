import { beforeEach, describe, expect, it, vi } from 'vitest';
import { followSystemTheme, hasManualTheme, initializeTheme, setTheme, theme } from './theme';

function mockMedia(matches: boolean) {
  vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({ matches, addEventListener: vi.fn() }));
}

describe('theme controls', () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
    localStorage.clear();
    document.documentElement.className = '';
    mockMedia(true);
    followSystemTheme();
  });

  it('follows operating system preference without a manual override', () => {
    initializeTheme();
    expect(theme.value).toBe('dark');
    expect(hasManualTheme.value).toBe(false);
    expect(document.documentElement.classList.contains('dark')).toBe(true);
  });

  it('persists and applies a manual light override', () => {
    initializeTheme();
    setTheme('light');
    expect(localStorage.getItem('jobs-theme')).toBe('light');
    expect(hasManualTheme.value).toBe(true);
    expect(document.documentElement.classList.contains('dark')).toBe(false);
  });

  it('can return to following the system preference', () => {
    setTheme('light');
    followSystemTheme();
    expect(localStorage.getItem('jobs-theme')).toBeNull();
    expect(hasManualTheme.value).toBe(false);
    expect(theme.value).toBe('dark');
  });
});

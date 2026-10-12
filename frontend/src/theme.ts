import { computed, ref } from 'vue';

export type Theme = 'light' | 'dark';
const storageKey = 'jobs-theme';
const manualOverride = ref(typeof localStorage !== 'undefined' && localStorage.getItem(storageKey) !== null);

function mediaQuery() {
  return typeof matchMedia !== 'undefined' ? matchMedia('(prefers-color-scheme: dark)') : null;
}

function storedTheme() {
  return typeof localStorage !== 'undefined' ? (localStorage.getItem(storageKey) as Theme | null) : null;
}

function systemTheme(): Theme {
  return mediaQuery()?.matches ? 'dark' : 'light';
}

export const theme = ref<Theme>(storedTheme() || systemTheme());
export const hasManualTheme = computed(() => manualOverride.value);

function apply(next: Theme) {
  theme.value = next;
  document.documentElement.classList.toggle('dark', next === 'dark');
}

export function setTheme(next: Theme) {
  localStorage.setItem(storageKey, next);
  manualOverride.value = true;
  apply(next);
}

export function followSystemTheme() {
  localStorage.removeItem(storageKey);
  manualOverride.value = false;
  apply(systemTheme());
}

export function initializeTheme() {
  const stored = storedTheme();
  manualOverride.value = stored !== null;
  apply(stored || systemTheme());
  mediaQuery()?.addEventListener('change', (event) => {
    if (!storedTheme()) apply(event.matches ? 'dark' : 'light');
  });
}

export const applyTheme = setTheme;
export const initTheme = initializeTheme;

import { readFile } from 'node:fs/promises';
import { resolve } from 'node:path';

const here = new URL('.', import.meta.url);

export const fixtures = JSON.parse(await readFile(new URL('./fixtures.json', here), 'utf8'));
export const defaultUser = fixtures.users[0];
export const defaultProfile = fixtures.profile;
export const defaultJobs = fixtures.jobs;

export const apiFailures = {
  signup: { status: 403, code: 'signups_disabled', message: 'Signups are disabled.' },
  verify: { status: 400, code: 'invalid_code', message: 'Invalid verification code.' },
  login: { status: 401, code: 'invalid_credentials', message: 'Invalid credentials.' },
  dashboard: { status: 500, code: 'jobs_failed', message: 'Dashboard unavailable.' },
  profileLoad: { status: 500, code: 'profile_failed', message: 'Profile unavailable.' },
  profileSave: { status: 500, code: 'profile_save_failed', message: 'Profile could not be saved.' },
  jobs: { status: 500, code: 'jobs_failed', message: 'Could not load jobs.' },
  jobDetail: { status: 500, code: 'job_failed', message: 'Could not load job match.' },
};

export function jsonResponse(body, status = 200) {
  return {
    status,
    contentType: 'application/json; charset=utf-8',
    body: JSON.stringify(body),
  };
}

export function errorResponse(failure) {
  return jsonResponse({ error: { code: failure.code, message: failure.message } }, failure.status);
}

export function makeBrowserApiState(overrides = {}) {
  return {
    authenticated: overrides.authenticated ?? true,
    csrfToken: overrides.csrfToken ?? 'csrf-preview',
    user: overrides.user ?? { email: defaultUser.email },
    profile: structuredClone(overrides.profile ?? defaultProfile),
    jobs: structuredClone(overrides.jobs ?? defaultJobs),
    fail: overrides.fail ?? {},
    calls: [],
  };
}

export async function installMockApi(page, state = makeBrowserApiState()) {
  await page.route('**/api/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    const bodyText = request.postData();
    const body = bodyText ? JSON.parse(bodyText) : undefined;
    state.calls.push({ path, method: request.method(), body });

    const failure = state.fail[path];
    if (failure) return route.fulfill(errorResponse(failure));

    if (path === '/api/session') {
      return route.fulfill(jsonResponse({
        authenticated: state.authenticated,
        csrfToken: state.csrfToken,
        user: state.authenticated ? state.user : undefined,
      }));
    }

    if (path === '/api/signup' && request.method() === 'POST') {
      return route.fulfill(jsonResponse({ next: 'verify', email: body.email }, 202));
    }

    if (path === '/api/verify' && request.method() === 'POST') {
      state.authenticated = true;
      return route.fulfill(jsonResponse({ next: 'onboarding', user: state.user }));
    }

    if (path === '/api/login' && request.method() === 'POST') {
      if (body.sendCode) return route.fulfill(jsonResponse({ status: 'code_sent' }, 202));
      state.authenticated = true;
      return route.fulfill(jsonResponse({ next: 'dashboard', user: state.user }));
    }

    if (path === '/api/logout' && request.method() === 'POST') {
      state.authenticated = false;
      return route.fulfill(jsonResponse({ status: 'logged_out' }));
    }

    if (path === '/api/dashboard') {
      return route.fulfill(jsonResponse({ user: state.user, profile: state.profile, recentJobs: state.jobs.slice(0, 1) }));
    }

    if (path === '/api/profile' && request.method() === 'GET') {
      return route.fulfill(jsonResponse({ user: state.user, profile: state.profile }));
    }

    if (path === '/api/profile' && request.method() === 'PUT') {
      state.profile = { ...state.profile, ...body, lastMatchRunMessage: defaultProfile.lastMatchRunMessage };
      return route.fulfill(jsonResponse({ profile: state.profile, message: state.profile.lastMatchRunMessage }));
    }

    if (path === '/api/jobs') return route.fulfill(jsonResponse({ jobs: state.jobs }));

    if (path.startsWith('/api/jobs/')) {
      const matchId = Number(path.split('/').pop());
      const job = state.jobs.find((candidate) => candidate.matchId === matchId);
      if (!job) return route.fulfill(errorResponse({ status: 404, code: 'job_not_found', message: 'Job match was not found.' }));
      return route.fulfill(jsonResponse({ job }));
    }

    return route.fulfill(errorResponse({ status: 404, code: 'not_found', message: `No fixture for ${path}` }));
  });

  return state;
}

export async function setThemeBeforeNavigation(page, theme) {
  await page.addInitScript((selectedTheme) => {
    localStorage.setItem('jobs-theme', selectedTheme);
  }, theme);
}

export function chromiumExecutablePath() {
  return process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || process.env.CHROMIUM_PATH || undefined;
}

export function projectPath(...parts) {
  return resolve(new URL('../..', here).pathname, ...parts);
}

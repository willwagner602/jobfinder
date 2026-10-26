import type { Page } from '@playwright/test';

export const defaultUser = { email: 'preview.user@example.test', verificationCode: '123456', loginCode: '654321' };

export const defaultProfile = {
  hasProfile: true,
  documentCount: 0,
  resumeText: 'Staff engineer with Vue, Go, and platform experience.',
  experienceText: 'Led API migrations\nImproved job matching reliability',
  desiredTitles: 'Staff Platform Engineer\nPrincipal Software Engineer',
  searchPreferences: 'Remote-first teams, product infrastructure, TypeScript and Go',
  blockedCompanies: 'Example BadCo',
  matcherName: 'default',
  lastMatchRunMessage: 'Saved; prior-week match run requested.',
};

export const defaultJobs = [
  {
    matchId: 101,
    listingId: 501,
    version: 1,
    title: 'Staff Platform Engineer',
    company: 'Northwind Labs',
    canonicalUrl: 'https://example.test/jobs/101',
    rating: 5,
    decision: 'match',
    createdAt: '2025-01-02T00:00:00Z',
    summary: 'Build reliable internal platforms for product engineering teams.',
  },
  {
    matchId: 102,
    listingId: 502,
    version: 1,
    title: 'Vue Application Engineer',
    company: 'Contoso Careers',
    canonicalUrl: 'https://example.test/jobs/102',
    rating: 4,
    decision: 'possible',
    createdAt: '2025-01-03T00:00:00Z',
    summary: 'Own accessible Vue interfaces for candidate workflows.',
  },
];

type BrowserApiFailure = { status: number; code: string; message: string };
export type BrowserApiCall = { path: string; method: string; body?: Record<string, unknown> };

type BrowserApiState = {
  authenticated: boolean;
  csrfToken: string;
  user: { email: string };
  profile: typeof defaultProfile;
  jobs: typeof defaultJobs;
  fail: Partial<Record<string, BrowserApiFailure>>;
  calls: BrowserApiCall[];
  documents: Array<{ id: number; filename: string; contentType: string; sizeBytes: number; createdAt: string }>;
};

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

function jsonResponse(body: unknown, status = 200) {
  return {
    status,
    contentType: 'application/json; charset=utf-8',
    body: JSON.stringify(body),
  };
}

function errorResponse(failure: BrowserApiFailure) {
  return jsonResponse({ error: { code: failure.code, message: failure.message } }, failure.status);
}

export function makeBrowserApiState(overrides: Partial<BrowserApiState> = {}): BrowserApiState {
  return {
    authenticated: overrides.authenticated ?? true,
    csrfToken: overrides.csrfToken ?? 'csrf-preview',
    user: overrides.user ?? { email: defaultUser.email },
    profile: structuredClone(overrides.profile ?? defaultProfile),
    jobs: structuredClone(overrides.jobs ?? defaultJobs),
    fail: overrides.fail ?? {},
    calls: [],
    documents: [],
  };
}

export async function installMockApi(page: Page, state = makeBrowserApiState()) {
  await page.route('**/api/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    const bodyText = request.postData();
    const body = bodyText && request.headers()['content-type']?.includes('application/json') ? JSON.parse(bodyText) as Record<string, unknown> : undefined;
    state.calls.push({ path, method: request.method(), body });

    const failure = state.fail[path];
    if (failure) return route.fulfill(errorResponse(failure));

    if (path === '/api/session') {
      return route.fulfill(jsonResponse({ authenticated: state.authenticated, csrfToken: state.csrfToken, user: state.authenticated ? state.user : undefined }));
    }
    if (path === '/api/signup' && request.method() === 'POST') return route.fulfill(jsonResponse({ next: 'verify', email: body?.email }, 202));
    if (path === '/api/verify' && request.method() === 'POST') {
      state.authenticated = true;
      return route.fulfill(jsonResponse({ next: 'onboarding', user: state.user }));
    }
    if (path === '/api/login' && request.method() === 'POST') {
      if (body?.sendCode) return route.fulfill(jsonResponse({ status: 'code_sent' }, 202));
      state.authenticated = true;
      return route.fulfill(jsonResponse({ next: 'dashboard', user: state.user }));
    }
    if (path === '/api/logout' && request.method() === 'POST') {
      state.authenticated = false;
      return route.fulfill(jsonResponse({ status: 'logged_out' }));
    }
    if (path === '/api/dashboard') return route.fulfill(jsonResponse({ user: state.user, profile: state.profile, recentJobs: state.jobs.slice(0, 1) }));
    if (path === '/api/profile' && request.method() === 'GET') return route.fulfill(jsonResponse({ user: state.user, profile: { ...state.profile, hasProfile: true, documentCount: state.documents.length } }));
    if (path === '/api/documents' && request.method() === 'GET') return route.fulfill(jsonResponse({ documents: state.documents }));
    if (path === '/api/documents' && request.method() === 'POST') {
      const document = { id: state.documents.length + 1, filename: 'resume.pdf', contentType: 'application/pdf', sizeBytes: 4, createdAt: '2026-01-01T00:00:00Z' };
      state.documents.push(document);
      return route.fulfill(jsonResponse({ document }, 201));
    }
    if (path === '/api/titles') return route.fulfill(jsonResponse({ items: state.profile.desiredTitles ? ['Software Engineer', 'Software Architect'] : [] }));
    if (path === '/api/companies') return route.fulfill(jsonResponse({ items: state.profile.blockedCompanies ? ['Example BadCo', 'NopeCo'] : [] }));
    if (path === '/api/profile' && request.method() === 'PUT') {
      const firstSave = !state.profile.hasProfile;
      state.profile = { ...state.profile, ...body, hasProfile: true, documentCount: state.documents.length, lastMatchRunMessage: firstSave ? 'Profile saved and initial match run requested.' : 'Profile saved.' };
      return route.fulfill(jsonResponse({ profile: state.profile, message: state.profile.lastMatchRunMessage }));
    }
    if (path === '/api/match-runs' && request.method() === 'POST') return route.fulfill(jsonResponse({ message: 'Prior-week match run requested.' }, 202));
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

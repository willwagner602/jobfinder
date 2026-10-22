import { vi } from 'vitest';

export const testUser = { email: 'preview.user@example.test' };

export const populatedProfile = {
  resumeText: 'Staff engineer with Vue, Go, and platform experience.',
  experienceText: 'Led API migrations\nImproved job matching reliability',
  desiredTitles: 'Staff Platform Engineer\nPrincipal Software Engineer',
  searchPreferences: 'Remote-first teams, product infrastructure, TypeScript and Go',
  blockedCompanies: 'Example BadCo',
  matcherName: 'default',
  lastMatchRunMessage: 'Saved; prior-week match run requested.',
};

export const populatedJobs = [
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

export type MockApiState = {
  authenticated?: boolean;
  csrfToken?: string;
  user?: { email: string };
  profile?: typeof populatedProfile;
  jobs?: typeof populatedJobs;
  fail?: Partial<Record<string, { status: number; code: string; message: string }>>;
};

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json; charset=utf-8' } });
}

export function installMockApi(overrides: MockApiState = {}) {
  const state = {
    authenticated: overrides.authenticated ?? true,
    csrfToken: overrides.csrfToken ?? 'csrf-preview',
    user: overrides.user ?? testUser,
    profile: overrides.profile ?? populatedProfile,
    jobs: overrides.jobs ?? populatedJobs,
    fail: overrides.fail ?? {},
  };
  const calls: Array<{ path: string; init?: RequestInit; body?: any }> = [];

  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input);
    const body = typeof init?.body === 'string' ? JSON.parse(init.body) : undefined;
    calls.push({ path, init, body });
    const failure = state.fail[path];
    if (failure && !(path === '/api/login' && body?.sendCode)) return json({ error: { code: failure.code, message: failure.message } }, failure.status);

    if (path === '/api/session') {
      return json({ authenticated: state.authenticated, csrfToken: state.csrfToken, user: state.authenticated ? state.user : undefined });
    }
    if (path === '/api/signup' && init?.method === 'POST') {
      return json({ next: 'verify', email: body.email }, 202);
    }
    if (path === '/api/verify' && init?.method === 'POST') {
      state.authenticated = true;
      return json({ next: 'onboarding', user: state.user });
    }
    if (path === '/api/login' && init?.method === 'POST') {
      if (body.sendCode) return json({ status: 'code_sent' }, 202);
      state.authenticated = true;
      return json({ next: 'dashboard', user: state.user });
    }
    if (path === '/api/logout' && init?.method === 'POST') {
      state.authenticated = false;
      return json({ status: 'logged_out' });
    }
    if (path === '/api/dashboard') return json({ user: state.user, profile: state.profile, recentJobs: state.jobs.slice(0, 1) });
    if (path === '/api/profile' && !init?.method) return json({ user: state.user, profile: state.profile });
    if (path === '/api/profile' && init?.method === 'PUT') {
      state.profile = { ...state.profile, ...body, lastMatchRunMessage: 'Saved; prior-week match run requested.' };
      return json({ profile: state.profile, message: state.profile.lastMatchRunMessage });
    }
    if (path === '/api/jobs') return json({ jobs: state.jobs });
    if (path.startsWith('/api/jobs/')) {
      const matchId = Number(path.split('/').pop());
      const job = state.jobs.find((candidate) => candidate.matchId === matchId);
      if (!job) return json({ error: { code: 'job_not_found', message: 'Job match was not found.' } }, 404);
      return json({ job });
    }
    return json({ error: { code: 'not_found', message: `No fixture for ${path}` } }, 404);
  });

  vi.stubGlobal('fetch', fetchMock);
  return { calls, fetchMock, state };
}

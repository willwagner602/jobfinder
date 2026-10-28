import './setup';
import { mount } from '@vue/test-utils';
import { describe, expect, it, vi } from 'vitest';
import App from '../src/App.vue';
import Dashboard from '../src/pages/Dashboard.vue';
import JobDetail from '../src/pages/JobDetail.vue';
import JobsList from '../src/pages/JobsList.vue';
import Login from '../src/pages/Login.vue';
import Profile from '../src/pages/Profile.vue';
import Signup from '../src/pages/Signup.vue';
import Verify from '../src/pages/Verify.vue';
import { createTestRouter } from './test-router';
import { installMockApi, populatedJobs } from './ui-fixtures';

async function mountWithRouter(component: any, path = '/', apiOptions = {}) {
  const router = createTestRouter(path);
  await router.push(path);
  const api = installMockApi(apiOptions);
  const wrapper = mount(component, { global: { plugins: [router] } });
  await router.isReady();
  await new Promise((resolve) => setTimeout(resolve, 0));
  return { wrapper, router, api };
}

describe('interactive component controls', () => {
  it('application shell exposes auth-aware navigation, theme controls, and logout failure feedback', async () => {
    const { wrapper } = await mountWithRouter(App, '/', {
      authenticated: true,
      fail: { '/api/logout': { status: 403, code: 'bad_csrf', message: 'Security token expired.' } },
    });
    const alerts: string[] = [];
    vi.stubGlobal('alert', (message: string) => alerts.push(message));

    expect(wrapper.get('a[aria-label="jobfinder home"]').attributes('href')).toBe('/');
    expect(wrapper.text()).toContain('Profile');
    expect(wrapper.text()).toContain('Job matches');
    await wrapper.get('button[title="Toggle light or dark theme"]').trigger('click');
    expect(localStorage.getItem('jobs-theme')).toBe('dark');
    await wrapper.get('button.secondary').trigger('click');
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(alerts).toEqual(['Security token expired.']);
  });

  it('signup form validates required email and renders server errors', async () => {
    const { wrapper } = await mountWithRouter(Signup, '/signup', {
      fail: { '/api/signup': { status: 403, code: 'signups_disabled', message: 'Signups are disabled.' } },
    });

    await wrapper.get('form').trigger('submit');
    expect(wrapper.text()).toContain('Email is required.');

    await wrapper.get('#signup-email').setValue('blocked@example.test');
    await wrapper.get('form').trigger('submit');
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(wrapper.text()).toContain('Signups are disabled.');
  });

  it('verification form validates email/code and renders invalid code errors', async () => {
    const { wrapper } = await mountWithRouter(Verify, '/verify?email=preview.user@example.test', {
      fail: { '/api/verify': { status: 400, code: 'invalid_code', message: 'Invalid verification code.' } },
    });

    expect(wrapper.text()).toContain("We've sent an email to");
    expect(wrapper.find('#verify-email').exists()).toBe(false);
    await wrapper.get('form').trigger('submit');
    expect(wrapper.text()).toContain('Email and verification code are required.');
    await wrapper.get('#verify-code').setValue('000000');
    await wrapper.get('form').trigger('submit');
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(wrapper.text()).toContain('Invalid verification code.');
    expect(wrapper.text()).toContain("We've sent an email to");
    expect(wrapper.find('#verify-email').exists()).toBe(false);
  });

  it('login form validates inputs, requests an emailed code, and renders credential errors', async () => {
    const failures = { '/api/login': { status: 401, code: 'invalid_credentials', message: 'Invalid credentials.' } };
    const { wrapper, api } = await mountWithRouter(Login, '/login', { fail: failures });

    await wrapper.get('form').trigger('submit');
    expect(wrapper.text()).toContain('Email is required.');

    await wrapper.get('#login-email').setValue('preview.user@example.test');
    await wrapper.get('form').trigger('submit');
    expect(wrapper.text()).toContain('Enter your password.');

    await wrapper.get('#login-password').setValue('bad-password');
    await wrapper.get('form').trigger('submit');
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(wrapper.text()).toContain('Invalid credentials.');

    await wrapper.get('button.code-option').trigger('click');
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(api.calls.find((call) => call.path === '/api/login' && call.body?.sendCode)?.body).toMatchObject({ sendCode: true });
    expect(wrapper.get('#login-code')).toBeTruthy();
    expect(wrapper.text()).toContain('A login code was sent if the account is ready.');
  });

  it('dashboard shows loading, empty, populated, and request failure states', async () => {
    const empty = await mountWithRouter(Dashboard, '/', { jobs: [], profile: { resumeText: '', experienceText: '' } });
    expect(empty.wrapper.text()).toContain('No matches yet');
    expect(empty.wrapper.get('a[href="/onboarding"]').text()).toContain('Update resume and experience');

    const populated = await mountWithRouter(Dashboard, '/');
    expect(populated.wrapper.text()).toContain(populatedJobs[0].title);

    const failed = await mountWithRouter(Dashboard, '/', {
      fail: { '/api/dashboard': { status: 500, code: 'jobs_failed', message: 'Dashboard unavailable.' } },
    });
    expect(failed.wrapper.text()).toContain('Dashboard unavailable.');
  });

  it('profile form shows document upload and saves selection failures', async () => {
    const { wrapper } = await mountWithRouter(Profile, '/onboarding', {
      profile: { resumeText: '', experienceText: '', desiredTitles: '', searchPreferences: '', blockedCompanies: '', matcherName: '', lastMatchRunMessage: '' },
      fail: { '/api/profile': { status: 500, code: 'profile_save_failed', message: 'Profile could not be saved.' } },
    });

    expect(wrapper.text()).toContain('Upload Documents');
    expect(wrapper.find('#resume-text').exists()).toBe(false);
    await wrapper.get('form').trigger('submit');
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(wrapper.text()).toContain('Profile could not be saved.');
  });

  it('job list renders populated, empty, and request failure states', async () => {
    const populated = await mountWithRouter(JobsList, '/jobs');
    expect(populated.wrapper.get(`a[aria-label="View ${populatedJobs[0].title} at ${populatedJobs[0].company}"]`).attributes('href')).toBe(`/jobs/${populatedJobs[0].matchId}`);

    const empty = await mountWithRouter(JobsList, '/jobs', { jobs: [] });
    expect(empty.wrapper.text()).toContain('No matches yet');
    expect(empty.wrapper.get('a[href="/onboarding"]').text()).toContain('Update profile');

    const failed = await mountWithRouter(JobsList, '/jobs', {
      fail: { '/api/jobs': { status: 500, code: 'jobs_failed', message: 'Could not load jobs.' } },
    });
    expect(failed.wrapper.text()).toContain('Could not load jobs.');
  });

  it('job detail renders back/original links, no-summary fallback, and not-found errors', async () => {
    const found = await mountWithRouter(JobDetail, `/jobs/${populatedJobs[0].matchId}`);
    expect(found.wrapper.get('a[href="/jobs"]').text()).toContain('Back to job matches');
    expect(found.wrapper.get('a[href="https://example.test/jobs/101"]').text()).toContain('Original listing');

    const noSummary = await mountWithRouter(JobDetail, '/jobs/101', { jobs: [{ ...populatedJobs[0], summary: '', canonicalUrl: '' }] });
    expect(noSummary.wrapper.text()).toContain('No summary was returned for this match.');

    const missing = await mountWithRouter(JobDetail, '/jobs/999');
    expect(missing.wrapper.text()).toContain('Job match was not found.');
  });
});

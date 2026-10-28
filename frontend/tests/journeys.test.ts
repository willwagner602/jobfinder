import './setup';
import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import App from '../src/App.vue';
import { session } from '../src/api';
import { createTestRouter } from './test-router';
import { installMockApi, populatedJobs, populatedProfile, testUser } from './ui-fixtures';

async function mountApp(path: string, authenticated = true) {
  const router = createTestRouter(path);
  await router.push(path);
  const api = installMockApi({ authenticated });
  const wrapper = mount(App, { global: { plugins: [router] } });
  await router.isReady();
  await wrapper.vm.$nextTick();
  await new Promise((resolve) => setTimeout(resolve, 0));
  return { wrapper, router, api };
}

describe('browser-level user journeys', () => {
  it('signs up, prefills verification, verifies email, and reaches onboarding', async () => {
    const { wrapper, router, api } = await mountApp('/signup', false);

    await wrapper.get('#signup-email').setValue(testUser.email);
    await wrapper.get('#signup-password').setValue('correct horse battery staple');
    await wrapper.get('form').trigger('submit');
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(api.calls.find((call) => call.path === '/api/signup')?.body).toEqual({
      email: testUser.email,
      password: 'correct horse battery staple',
    });
    expect(router.currentRoute.value.path).toBe('/verify');
    expect(wrapper.get('[data-testid="verification-email-notice"]').text()).toContain(`We've sent an email to ${testUser.email}`);
    expect(wrapper.find('#verify-email').exists()).toBe(false);

    await wrapper.get('#verify-code').setValue('123456');
    await wrapper.get('form').trigger('submit');
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(api.calls.find((call) => call.path === '/api/verify')?.body).toEqual({ email: testUser.email, code: '123456' });
    expect(router.currentRoute.value.path).toBe('/onboarding');
    expect(session.authenticated).toBe(true);
  });

  it('logs in, logs out, and updates visible navigation', async () => {
    const { wrapper, router, api } = await mountApp('/login', false);

    await wrapper.get('#login-email').setValue(testUser.email);
    await wrapper.get('#login-password').setValue('password');
    await wrapper.get('form').trigger('submit');
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(api.calls.find((call) => call.path === '/api/login')?.body).toMatchObject({ email: testUser.email, password: 'password' });
    expect(router.currentRoute.value.path).toBe('/dashboard');
    expect(wrapper.text()).toContain('Dashboard');
    expect(wrapper.text()).toContain('Log out');

    await wrapper.get('button.secondary').trigger('click');
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(api.calls.some((call) => call.path === '/api/logout')).toBe(true);
    expect(router.currentRoute.value.path).toBe('/login');
    expect(wrapper.text()).toContain('Sign up');
  });

  it('saves title and company selections without text boxes', async () => {
    const { wrapper, api } = await mountApp('/onboarding');

    expect(wrapper.text()).toContain('Upload Documents');
    expect(wrapper.find('#resume-text').exists()).toBe(false);
    await wrapper.get('#title-search').setValue('Software');
    await new Promise((resolve) => setTimeout(resolve, 180));
    await wrapper.get('[role="option"]').trigger('click');
    await wrapper.get('#company-search').setValue('Example');
    await new Promise((resolve) => setTimeout(resolve, 180));
    await wrapper.get('[role="option"]').trigger('click');
    await wrapper.get('form').trigger('submit');
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(api.calls.find((call) => call.path === '/api/profile' && call.init?.method === 'PUT')?.body).toMatchObject({
      desiredTitles: 'Staff Platform Engineer\nPrincipal Software Engineer\nSoftware Engineer',
      blockedCompanies: 'Example BadCo',
    });
    expect(wrapper.text()).toContain('Profile saved.');
  });

  it('shows dashboard links and navigates through jobs list and detail pages', async () => {
    const { wrapper, router } = await mountApp('/dashboard');

    expect(wrapper.text()).toContain(`Welcome, ${testUser.email}`);
    await wrapper.get('a[href="/jobs"]').trigger('click');
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(router.currentRoute.value.path).toBe('/jobs');
    expect(wrapper.text()).toContain(populatedJobs[0].title);

    await wrapper.get(`a[aria-label="View ${populatedJobs[0].title} at ${populatedJobs[0].company}"]`).trigger('click');
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(router.currentRoute.value.path).toBe(`/jobs/${populatedJobs[0].matchId}`);
    expect(wrapper.text()).toContain('Original listing');
    expect(wrapper.text()).toContain(populatedJobs[0].summary);
  });

  it('changes theme manually and returns to following system preference', async () => {
    const { wrapper } = await mountApp('/dashboard');

    await wrapper.get('button[title="Toggle light or dark theme"]').trigger('click');
    expect(localStorage.getItem('jobs-theme')).toBe('dark');
    expect(document.documentElement.classList.contains('dark')).toBe(true);
    expect(wrapper.text()).toContain('Use system theme');

    await wrapper.get('button.ghost').trigger('click');
    expect(localStorage.getItem('jobs-theme')).toBeNull();
  });
});

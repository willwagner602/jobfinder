import { expect, test } from '@playwright/test';
import {
  apiFailures,
  defaultJobs,
  defaultProfile,
  defaultUser,
  installMockApi,
  makeBrowserApiState,
} from './browser-fixtures';

test.describe('browser-level user journeys', () => {
  test('signs up, prefills verification, verifies email, and reaches onboarding', async ({ page }) => {
    const state = await installMockApi(page, makeBrowserApiState({ authenticated: false }));

    await page.goto('/signup');
    await page.getByLabel('Email').fill(defaultUser.email);
    await page.getByLabel(/Password/).fill('correct horse battery staple');
    await page.getByRole('button', { name: 'Create account and send code' }).click();

    await expect(page).toHaveURL(/\/verify\?email=preview\.user@example\.test/);
    await expect(page.getByTestId('verification-email-notice')).toContainText(`We've sent an email to ${defaultUser.email}`);
    await expect(page.getByLabel('Email')).toHaveCount(0);
    expect(state.calls.find((call) => call.path === '/api/signup')?.body).toMatchObject({
      email: defaultUser.email,
      password: 'correct horse battery staple',
    });

    await page.getByLabel('One-time code').fill(defaultUser.verificationCode);
    await page.getByRole('button', { name: 'Verify and continue' }).click();

    await expect(page).toHaveURL(/\/onboarding$/);
    await expect(page.getByLabel('Upload Documents')).toBeVisible();
    expect(state.calls.find((call) => call.path === '/api/verify')?.body).toEqual({
      email: defaultUser.email,
      code: defaultUser.verificationCode,
    });
  });

  test('logs in, logs out, and updates visible navigation', async ({ page }) => {
    const state = await installMockApi(page, makeBrowserApiState({ authenticated: false }));

    await page.goto('/login');
    await expect(page.getByLabel('One-time code')).toHaveCount(0);
    await page.getByLabel('Email').fill(defaultUser.email);
    await page.getByLabel('Password', { exact: true }).fill('password');
    await page.getByRole('button', { name: 'Log in', exact: true }).click();

    await expect(page).toHaveURL(/\/dashboard$/);
    await expect(page.getByRole('link', { name: 'Dashboard', exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: /Log out/ })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Update resume and experience' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'View job matches' })).toBeVisible();
    expect(state.calls.find((call) => call.path === '/api/login')?.body).toMatchObject({ email: defaultUser.email, password: 'password' });

    await page.getByRole('button', { name: /Log out/ }).click();
    await expect(page).toHaveURL(/\/login$/);
    await expect(page.getByRole('link', { name: 'Sign up' })).toBeVisible();
    expect(state.calls.some((call) => call.path === '/api/logout')).toBe(true);
  });

  test('requests an emailed login code and shows delivery status', async ({ page }) => {
    const state = await installMockApi(page, makeBrowserApiState({ authenticated: false }));

    await page.goto('/login');
    await expect(page.getByLabel('One-time code')).toHaveCount(0);
    await page.getByLabel('Email').fill(defaultUser.email);
    await page.getByRole('button', { name: 'Send me a one time code' }).click();

    await expect(page.getByRole('status')).toContainText('A login code was sent if the account is ready.');
    await expect(page.getByLabel('One-time code')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Use password instead' })).toBeVisible();
    expect(state.calls.find((call) => call.path === '/api/login' && call.body?.sendCode)?.body).toMatchObject({
      email: defaultUser.email,
      sendCode: true,
    });

    await page.getByLabel('One-time code').fill(defaultUser.loginCode);
    await page.getByRole('button', { name: 'Log in', exact: true }).click();
    await expect(page).toHaveURL(/\/dashboard$/);
    expect(state.calls.find((call) => call.path === '/api/login' && call.body?.code)?.body).toMatchObject({
      email: defaultUser.email,
      code: defaultUser.loginCode,
    });
  });

  test('redirects unauthenticated visitors from onboarding to login', async ({ page }) => {
    const state = await installMockApi(page, makeBrowserApiState({ authenticated: false }));

    for (const path of ['/dashboard', '/onboarding', '/jobs', '/jobs/101']) {
      await page.goto(path);
      await expect(page).toHaveURL(new RegExp(`/login\\?redirect=${path.replaceAll('/', '\\/')}$`));
      await expect(page.getByRole('heading', { name: 'Log in', exact: true })).toBeVisible();
    }
    expect(state.calls.some((call) => ['/api/profile', '/api/dashboard', '/api/jobs'].includes(call.path))).toBe(false);
  });

  test('uploads documents, edits title and company selections, and saves the profile', async ({ page }) => {
    const state = await installMockApi(page, makeBrowserApiState());

    await page.goto('/onboarding');
    await expect(page.getByLabel('Upload Documents')).toBeVisible();
    await expect(page.getByLabel('Resume text')).toHaveCount(0);
    await page.getByLabel('Desired titles').fill('Software');
    await expect(page.getByRole('option', { name: 'Software Engineer' })).toBeVisible();
    await page.getByRole('option', { name: 'Software Engineer' }).click();
    await page.getByLabel('Blocked companies').fill('Nope');
    await expect(page.getByRole('option', { name: 'NopeCo' })).toBeVisible();
    await page.getByRole('option', { name: 'NopeCo' }).click();
    await page.getByLabel('Upload Documents').setInputFiles({ name: 'resume.pdf', mimeType: 'application/pdf', buffer: Buffer.from('%PDF') });
    await expect(page.getByText(/resume\.pdf/)).toBeVisible();
    await page.getByRole('button', { name: 'Save profile' }).click();

    await expect(page.getByRole('status')).toContainText('Profile saved.');
    expect(state.calls.find((call) => call.path === '/api/profile' && call.method === 'PUT')?.body).toMatchObject({
      desiredTitles: 'Staff Platform Engineer\nPrincipal Software Engineer\nSoftware Engineer',
      blockedCompanies: 'Example BadCo\nNopeCo',
    });
    expect(state.calls.some((call) => call.path === '/api/documents' && call.method === 'POST')).toBe(true);
  });

  test('shows dashboard links and navigates through jobs list and detail pages', async ({ page }) => {
    await installMockApi(page, makeBrowserApiState({ authenticated: true }));

    await page.goto('/dashboard');
    await expect(page.getByRole('heading', { name: new RegExp(`Welcome,\\s*${defaultUser.email}`) })).toBeVisible();
    await page.getByRole('link', { name: 'View job matches' }).click();

    await expect(page).toHaveURL(/\/jobs$/);
    await expect(page.getByRole('link', { name: `View ${defaultJobs[0].title} at ${defaultJobs[0].company}` })).toBeVisible();
    await page.getByRole('link', { name: `View ${defaultJobs[0].title} at ${defaultJobs[0].company}` }).click();

    await expect(page).toHaveURL(new RegExp(`/jobs/${defaultJobs[0].matchId}$`));
    await expect(page.getByRole('link', { name: 'Original listing' })).toBeVisible();
    await expect(page.getByText(defaultJobs[0].summary)).toBeVisible();
  });

  test('shows the public landing page and navigates toward signup', async ({ page }) => {
    await installMockApi(page, makeBrowserApiState({ authenticated: false }));

    await page.goto('/');
    await expect(page.getByRole('heading', { name: /Welcome/ })).toBeVisible();
    await expect(page.getByTestId('info-section')).toBeVisible();
    await expect(page.getByTestId('landing-actions')).toBeVisible();

    await page.getByRole('link', { name: 'Create an account' }).click();
    await expect(page).toHaveURL(/\/signup$/);
  });

  test('changes theme manually and returns to following system preference', async ({ page }) => {
    await installMockApi(page, makeBrowserApiState());

    await page.goto('/');
    await page.getByRole('button', { name: /following system \(light\)|light theme selected/ }).click();
    await expect(page.locator('html')).toHaveClass(/dark/);
    await expect(page.getByRole('button', { name: 'Use system theme' })).toBeVisible();
    expect(await page.evaluate(() => localStorage.getItem('jobs-theme'))).toBe('dark');

    await page.getByRole('button', { name: 'Use system theme' }).click();
    expect(await page.evaluate(() => localStorage.getItem('jobs-theme'))).toBeNull();
  });

  test('renders validation and server failures in browser-visible alerts', async ({ page }) => {
    await installMockApi(page, makeBrowserApiState({
      authenticated: false,
      fail: {
        '/api/signup': apiFailures.signup,
        '/api/verify': apiFailures.verify,
        '/api/login': apiFailures.login,
      },
    }));

    await page.goto('/signup');
    await page.getByRole('button', { name: 'Create account and send code' }).click();
    await expect(page.getByLabel('Email')).toHaveJSProperty('validity.valid', false);
    await page.getByLabel('Email').fill('blocked@example.test');
    await page.getByRole('button', { name: 'Create account and send code' }).click();
    await expect(page.getByRole('alert')).toContainText('Signups are disabled.');

    await page.goto(`/verify?email=${defaultUser.email}`);
    await page.getByRole('button', { name: 'Verify and continue' }).click();
    await expect(page.getByLabel('One-time code')).toHaveJSProperty('validity.valid', false);
    await page.getByLabel('One-time code').fill('000000');
    await page.getByRole('button', { name: 'Verify and continue' }).click();
    await expect(page.getByRole('alert')).toContainText('Invalid verification code.');

    await page.goto('/login');
    await expect(page.getByLabel('One-time code')).toHaveCount(0);
    await page.getByRole('button', { name: 'Log in', exact: true }).click();
    await expect(page.getByLabel('Email')).toHaveJSProperty('validity.valid', false);
    await page.getByLabel('Email').fill(defaultUser.email);
    await page.getByRole('button', { name: 'Log in', exact: true }).click();
    await expect(page.getByRole('alert')).toContainText('Enter your password.');
    await page.getByLabel('Password', { exact: true }).fill('bad-password');
    await page.getByRole('button', { name: 'Log in', exact: true }).click();
    await expect(page.getByRole('alert')).toContainText('Invalid credentials.');
  });
});

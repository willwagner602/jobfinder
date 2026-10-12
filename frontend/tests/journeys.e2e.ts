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
    await expect(page.getByLabel('Email')).toHaveValue(defaultUser.email);
    expect(state.calls.find((call) => call.path === '/api/signup')?.body).toMatchObject({
      email: defaultUser.email,
      password: 'correct horse battery staple',
    });

    await page.getByLabel('One-time code').fill(defaultUser.verificationCode);
    await page.getByRole('button', { name: 'Verify and continue' }).click();

    await expect(page).toHaveURL(/\/onboarding$/);
    await expect(page.getByLabel('Resume text')).toHaveValue(defaultProfile.resumeText);
    expect(state.calls.find((call) => call.path === '/api/verify')?.body).toEqual({
      email: defaultUser.email,
      code: defaultUser.verificationCode,
    });
  });

  test('logs in, logs out, and updates visible navigation', async ({ page }) => {
    const state = await installMockApi(page, makeBrowserApiState({ authenticated: false }));

    await page.goto('/login');
    await page.getByLabel('Email').fill(defaultUser.email);
    await page.getByLabel('Password').fill('password');
    await page.getByRole('button', { name: 'Log in', exact: true }).click();

    await expect(page).toHaveURL(/\/$/);
    await expect(page.getByRole('link', { name: 'Dashboard', exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: /Log out/ })).toBeVisible();
    expect(state.calls.find((call) => call.path === '/api/login')?.body).toMatchObject({ email: defaultUser.email, password: 'password' });

    await page.getByRole('button', { name: /Log out/ }).click();
    await expect(page).toHaveURL(/\/login$/);
    await expect(page.getByRole('link', { name: 'Sign up' })).toBeVisible();
    expect(state.calls.some((call) => call.path === '/api/logout')).toBe(true);
  });

  test('requests an emailed login code and shows delivery status', async ({ page }) => {
    const state = await installMockApi(page, makeBrowserApiState({ authenticated: false }));

    await page.goto('/login');
    await page.getByLabel('Email').fill(defaultUser.email);
    await page.getByRole('button', { name: 'Email me a code' }).click();

    await expect(page.getByRole('status')).toContainText('A login code was sent if the account is ready.');
    expect(state.calls.find((call) => call.path === '/api/login' && call.body?.sendCode)?.body).toMatchObject({
      email: defaultUser.email,
      sendCode: true,
    });
  });

  test('edits profile/onboarding fields and shows the saved server message', async ({ page }) => {
    const state = await installMockApi(page, makeBrowserApiState());

    await page.goto('/onboarding');
    await expect(page.getByLabel('Resume text')).toHaveValue(defaultProfile.resumeText);
    await page.getByLabel('Resume text').fill('Updated resume text');
    await page.getByLabel('Experience bullets').fill('Built deterministic browser tests');
    await page.getByLabel('Desired titles, one per line').fill('Staff Engineer');
    await page.getByLabel('Search preferences JSON or notes').fill('Remote only');
    await page.getByLabel('Blocked companies, one per line').fill('NopeCo');
    await page.getByLabel('Matcher override').fill('default');
    await page.getByRole('button', { name: 'Save and request prior-week match run' }).click();

    await expect(page.getByRole('status')).toContainText(defaultProfile.lastMatchRunMessage);
    expect(state.calls.find((call) => call.path === '/api/profile' && call.method === 'PUT')?.body).toMatchObject({
      resumeText: 'Updated resume text',
      experienceText: 'Built deterministic browser tests',
      desiredTitles: 'Staff Engineer',
    });
  });

  test('shows dashboard links and navigates through jobs list and detail pages', async ({ page }) => {
    await installMockApi(page, makeBrowserApiState());

    await page.goto('/');
    await expect(page.getByRole('heading', { name: `Welcome, ${defaultUser.email}.` })).toBeVisible();
    await page.getByRole('link', { name: 'View job matches' }).click();

    await expect(page).toHaveURL(/\/jobs$/);
    await expect(page.getByRole('link', { name: `View ${defaultJobs[0].title} at ${defaultJobs[0].company}` })).toBeVisible();
    await page.getByRole('link', { name: `View ${defaultJobs[0].title} at ${defaultJobs[0].company}` }).click();

    await expect(page).toHaveURL(new RegExp(`/jobs/${defaultJobs[0].matchId}$`));
    await expect(page.getByRole('link', { name: 'Original listing' })).toBeVisible();
    await expect(page.getByText(defaultJobs[0].summary)).toBeVisible();
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
    await page.getByRole('button', { name: 'Log in', exact: true }).click();
    await expect(page.getByLabel('Email')).toHaveJSProperty('validity.valid', false);
    await page.getByLabel('Email').fill(defaultUser.email);
    await page.getByRole('button', { name: 'Log in', exact: true }).click();
    await expect(page.getByRole('alert')).toContainText('Enter a password or one-time code.');
    await page.getByLabel('Password').fill('bad-password');
    await page.getByRole('button', { name: 'Log in', exact: true }).click();
    await expect(page.getByRole('alert')).toContainText('Invalid credentials.');
  });
});

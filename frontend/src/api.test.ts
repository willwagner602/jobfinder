import { beforeEach, describe, expect, it, vi } from 'vitest';
import { apiSend, errorMessage, refreshSession, session } from './api';

describe('api helpers', () => {
  beforeEach(() => { session.csrfToken=''; session.authenticated=false; session.loading=true; vi.restoreAllMocks(); });
  it('refreshes session and sends CSRF token with credentials for mutations', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ authenticated: true, csrfToken: 'csrf-1', user: { email: 'u@example.com' } }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ ok: true }), { status: 200, headers: { 'Content-Type': 'application/json' } }));
    vi.stubGlobal('fetch', fetchMock);
    await refreshSession();
    await apiSend('/api/profile', 'PUT', { resumeText: 'resume' });
    expect(fetchMock).toHaveBeenLastCalledWith('/api/profile', expect.objectContaining({ method: 'PUT', credentials: 'include', headers: expect.objectContaining({ 'X-CSRF-Token': 'csrf-1' }) }));
  });
  it('returns server error messages', async () => {
    const err = new Error('Server said no');
    expect(errorMessage(err)).toBe('Server said no');
  });
});

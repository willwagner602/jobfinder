import { reactive } from 'vue';

export type User = { email: string };
export type Profile = {
  resumeText?: string;
  experienceText?: string;
  desiredTitles?: string;
  searchPreferences?: string;
  blockedCompanies?: string;
  matcherName?: string;
  lastMatchRunMessage?: string;
  hasProfile?: boolean;
  documentCount?: number;
  documents?: DocumentInfo[];
};
export type DocumentInfo = { id: number; filename: string; contentType: string; sizeBytes: number; createdAt: string };
export type Job = {
  matchId: number;
  listingId?: number;
  version?: number;
  title: string;
  company: string;
  canonicalUrl?: string;
  rating?: number | null;
  decision: string;
  createdAt?: string;
  summary?: string;
};
export type SessionState = { loading: boolean; authenticated: boolean; csrfToken: string; user?: User };
export type ApiError = Error & { status?: number; code?: string };

export const session = reactive<SessionState>({ loading: true, authenticated: false, csrfToken: '' });

export function errorMessage(error: unknown): string {
  if (error instanceof Error) return error.message;
  return 'Something went wrong. Please try again.';
}

async function parseResponse<T>(response: Response): Promise<T> {
  const data = await response.json().catch(() => ({}));
  if (!response.ok) {
    const err = new Error(data?.error?.message || 'Request failed. Please try again.') as ApiError;
    err.status = response.status;
    err.code = data?.error?.code;
    throw err;
  }
  return data as T;
}

export async function refreshSession(): Promise<SessionState> {
  session.loading = true;
  const data = await parseResponse<{ authenticated: boolean; csrfToken: string; user?: User }>(
    await fetch('/api/session', { credentials: 'include' }),
  );
  session.authenticated = data.authenticated;
  session.csrfToken = data.csrfToken;
  session.user = data.user;
  session.loading = false;
  return session;
}

export async function apiGet<T>(path: string): Promise<T> {
  return parseResponse<T>(await fetch(path, { credentials: 'include' }));
}

export async function csrfToken(): Promise<string> {
  if (!session.csrfToken) await refreshSession();
  return session.csrfToken;
}

export async function apiSend<T>(path: string, method: 'POST' | 'PUT', body: unknown): Promise<T> {
  if (!session.csrfToken) await refreshSession();
  return parseResponse<T>(
    await fetch(path, {
      method,
      credentials: 'include',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': session.csrfToken },
      body: JSON.stringify(body),
    }),
  );
}

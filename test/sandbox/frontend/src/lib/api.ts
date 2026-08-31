// AUTO-GENERATED from the API contract (openapi.yaml). Do NOT edit by hand.
// Import these typed functions in your components — never hardcode fetch('/api/...').
// Every request goes through the Vite /api proxy to the backend and attaches the
// JWT from localStorage (set it after login: localStorage.setItem('token', token)).

export function authHeaders(): Record<string, string> {
  const t = typeof localStorage !== 'undefined' ? localStorage.getItem('token') : null;
  return t ? { Authorization: `Bearer ${t}` } : {};
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: { 'Content-Type': 'application/json', ...authHeaders() },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) throw new Error(`${method} ${path} -> ${res.status}`);
  const text = await res.text();
  return (text ? JSON.parse(text) : (null as unknown)) as T;
}

function qs(q?: Record<string, string | number | boolean | undefined>): string {
  if (!q) return '';
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(q)) if (v !== undefined) p.set(k, String(v));
  const s = p.toString();
  return s ? `?${s}` : '';
}

export interface createV1AuthSignupResult {
  id: number;
  username: string;
  email: string;
  bio: string;
  created_at: string;
  tokens: string;
}
export function createV1AuthSignup(body: { username: string; email: string; password: string }): Promise<createV1AuthSignupResult> {
  return request<createV1AuthSignupResult>('POST', `/api/v1/auth/signup`, body);
}

export interface createV1AuthLoginResult {
  id: number;
  username: string;
  email: string;
  bio: string;
  created_at: string;
  tokens: string;
}
export function createV1AuthLogin(body: { username: string; password: string }): Promise<createV1AuthLoginResult> {
  return request<createV1AuthLoginResult>('POST', `/api/v1/auth/login`, body);
}

export interface createV1AuthRefreshResult {
  tokens: string;
}
export function createV1AuthRefresh(): Promise<createV1AuthRefreshResult> {
  return request<createV1AuthRefreshResult>('POST', `/api/v1/auth/refresh`, undefined);
}

export function createV1AuthLogout(): Promise<void> {
  return request<void>('POST', `/api/v1/auth/logout`, undefined);
}

export interface getV1MeResult {
  id: number;
  username: string;
  email: string;
  bio: string;
  created_at: string;
  post_count: number;
  total_likes_received: number;
}
export function getV1Me(query?: Record<string, string | number | boolean | undefined>): Promise<getV1MeResult> {
  return request<getV1MeResult>('GET', `/api/v1/me` + qs(query), undefined);
}

export interface updateV1MeResult {
  id: number;
  username: string;
  email: string;
  bio: string;
  created_at: string;
  post_count: number;
  total_likes_received: number;
}
export function updateV1Me(body: { bio: string }): Promise<updateV1MeResult> {
  return request<updateV1MeResult>('PATCH', `/api/v1/me`, body);
}

export interface listV1FeedResult {
  id: number;
  author_id: number;
  body: string;
  created_at: string;
  post_date: string;
  like_count: number;
  is_deleted: boolean;
  author_username: string;
}
export function listV1Feed(query?: Record<string, string | number | boolean | undefined>): Promise<listV1FeedResult[]> {
  return request<listV1FeedResult[]>('GET', `/api/v1/feed` + qs(query), undefined);
}

export interface createV1PostsResult {
  id: number;
  author_id: number;
  body: string;
  created_at: string;
  post_date: string;
  like_count: number;
  is_deleted: boolean;
  author_username: string;
}
export function createV1Posts(body: { body: string }): Promise<createV1PostsResult> {
  return request<createV1PostsResult>('POST', `/api/v1/posts`, body);
}

export interface getV1PostsByIdResult {
  id: number;
  author_id: number;
  body: string;
  created_at: string;
  post_date: string;
  like_count: number;
  is_deleted: boolean;
  author_username: string;
}
export function getV1PostsById(id: string | number, query?: Record<string, string | number | boolean | undefined>): Promise<getV1PostsByIdResult> {
  return request<getV1PostsByIdResult>('GET', `/api/v1/posts/${id}` + qs(query), undefined);
}

export interface removeV1PostsByIdResult {
  id: number;
  body: string;
  created_at: string;
  is_deleted: boolean;
}
export function removeV1PostsById(id: string | number): Promise<removeV1PostsByIdResult> {
  return request<removeV1PostsByIdResult>('DELETE', `/api/v1/posts/${id}`, undefined);
}

export interface createV1PostsLikeResult {
  post_id: number;
  like_count: number;
}
export function createV1PostsLike(id: string | number): Promise<createV1PostsLikeResult> {
  return request<createV1PostsLikeResult>('POST', `/api/v1/posts/${id}/like`, undefined);
}

export interface removeV1PostsLikeResult {
  post_id: number;
  like_count: number;
}
export function removeV1PostsLike(id: string | number): Promise<removeV1PostsLikeResult> {
  return request<removeV1PostsLikeResult>('DELETE', `/api/v1/posts/${id}/like`, undefined);
}

export interface getV1UsersByUsernameResult {
  id: number;
  username: string;
  email: string;
  bio: string;
  created_at: string;
  post_count: number;
  total_likes_received: number;
}
export function getV1UsersByUsername(username: string | number, query?: Record<string, string | number | boolean | undefined>): Promise<getV1UsersByUsernameResult> {
  return request<getV1UsersByUsernameResult>('GET', `/api/v1/users/${username}` + qs(query), undefined);
}

export interface listV1UsersPostsResult {
  id: number;
  author_id: number;
  body: string;
  created_at: string;
  post_date: string;
  like_count: number;
  is_deleted: boolean;
  author_username: string;
}
export function listV1UsersPosts(username: string | number, query?: Record<string, string | number | boolean | undefined>): Promise<listV1UsersPostsResult[]> {
  return request<listV1UsersPostsResult[]>('GET', `/api/v1/users/${username}/posts` + qs(query), undefined);
}

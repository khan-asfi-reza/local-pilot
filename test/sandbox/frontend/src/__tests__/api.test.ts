import { describe, it, expect, vi, beforeEach } from 'vitest';
import * as api from '../lib/api';

// Mock fetch globally
const mockFetch = vi.fn();
global.fetch = mockFetch;

describe('API Client', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockFetch.mockReset();
  });

  describe('request helper', () => {
    it('makes requests with correct headers', async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        json: async () => ({ data: 'test' }),
      });

      const result = await api.request('GET', '/api/test');

      expect(mockFetch).toHaveBeenCalledWith('/api/test', expect.any(Object));
      const callArgs = mockFetch.mock.calls[0][1] as RequestInit;
      expect(callArgs.headers).toBeDefined();
      expect(callArgs.headers['Content-Type']).toBe('application/json');
    });

    it('handles non-OK responses', async () => {
      mockFetch.mockResolvedValue({
        ok: false,
        status: 404,
        json: async () => ({ error: 'Not found' }),
      });

      try {
        await api.request('GET', '/api/notfound');
        expect(true).toBe(false); // Should not reach here
      } catch (error) {
        const err = error as any;
        expect(err.message).toContain('404');
        expect(err.response?.data).toEqual({ error: 'Not found' });
      }
    });

    it('handles JSON parse errors', async () => {
      mockFetch.mockResolvedValue({
        ok: true,
        status: 200,
        json: async () => { throw new Error('Invalid JSON'); },
      });

      try {
        await api.request('GET', '/api/invalid-json');
        expect(true).toBe(false);
      } catch (error) {
        const err = error as any;
        expect(err.message).toContain('Failed to parse response');
      }
    });
  });

  describe('auth headers', () => {
    it('returns correct auth header format', () => {
      const headers = api.authHeaders();
      expect(headers['Authorization']).toBeDefined();
      expect(headers['Content-Type']).toBe('application/json');
    });
  });

  describe('query string builder', () => {
    it('builds empty query string for no params', () => {
      expect(api.qs({})).toBe('');
    });

    it('builds query string with single param', () => {
      expect(api.qs({ page: 1 })).toBe('page=1');
    });

    it('builds query string with multiple params', () => {
      expect(api.qs({ page: 1, limit: 10 })).toBe('page=1&limit=10');
    });

    it('handles boolean values', () => {
      expect(api.qs({ active: true })).toBe('active=true');
      expect(api.qs({ active: false })).toBe('active=false');
    });

    it('handles undefined values by omitting them', () => {
      const qs = api.qs({ page: 1, limit: undefined } as any);
      expect(qs).toBe('page=1');
    });
  });

  describe('Auth endpoints', () => {
    let mockToken: string;

    beforeEach(() => {
      mockToken = 'mock-jwt-token-12345';
      mockFetch.mockResolvedValue({
        ok: true,
        json: async () => ({ tokens: { access_token: mockToken } }),
      });
    });

    it('creates signup request', async () => {
      const result = await api.createV1AuthSignup({
        username: 'testuser',
        email: 'test@example.com',
        password: 'securepass',
      });

      expect(mockFetch).toHaveBeenCalledWith('/api/v1/auth/signup', expect.any(Object));
      const callArgs = mockFetch.mock.calls[0][1] as RequestInit;
      expect(callArgs.body).toContain('username=testuser');
      expect(callArgs.body).toContain('email=test@example.com');
      expect(callArgs.body).toContain('password=securepass');

      expect(result.username).toBe('testuser');
      expect(result.email).toBe('test@example.com');
    });

    it('creates login request', async () => {
      const result = await api.createV1AuthLogin({
        username: 'testuser',
        password: 'securepass',
      });

      expect(mockFetch).toHaveBeenCalledWith('/api/v1/auth/login', expect.any(Object));
      const callArgs = mockFetch.mock.calls[0][1] as RequestInit;
      expect(callArgs.body).toContain('username=testuser');
      expect(callArgs.body).toContain('password=securepass');

      expect(result.tokens.access_token).toBe(mockToken);
    });

    it('creates refresh request', async () => {
      const result = await api.createV1AuthRefresh();

      expect(mockFetch).toHaveBeenCalledWith('/api/v1/auth/refresh', expect.any(Object));
      expect(result.tokens.access_token).toBeDefined();
    });

    it('creates logout request', async () => {
      await api.createV1AuthLogout();

      expect(mockFetch).toHaveBeenCalledWith('/api/v1/auth/logout', expect.any(Object));
    });
  });

  describe('Me endpoint', () => {
    const mockUser = {
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: 'Test bio',
      created_at: new Date().toISOString(),
      post_count: 5,
      total_likes_received: 42,
    };

    beforeEach(() => {
      mockFetch.mockResolvedValue({ ok: true, json: async () => mockUser });
    });

    it('fetches current user', async () => {
      const result = await api.getV1Me();

      expect(mockFetch).toHaveBeenCalledWith('/api/v1/me', expect.any(Object));
      expect(result.username).toBe('testuser');
      expect(result.email).toBe('test@example.com');
    });

    it('updates current user', async () => {
      const result = await api.updateV1Me({ bio: 'Updated bio' });

      expect(mockFetch).toHaveBeenCalledWith('/api/v1/me', {
        method: 'PATCH',
        headers: expect.any(Object),
        body: expect.any(String),
      });
      expect(result.bio).toBe('Updated bio');
    });
  });

  describe('Feed endpoint', () => {
    const mockPosts = [
      {
        id: 1,
        author_id: 1,
        body: 'Hello world!',
        created_at: new Date().toISOString(),
        post_date: '2024-01-01',
        like_count: 5,
        is_deleted: false,
        author_username: 'alice',
      },
      {
        id: 2,
        author_id: 2,
        body: 'Second post',
        created_at: new Date().toISOString(),
        post_date: '2024-01-01',
        like_count: 3,
        is_deleted: false,
        author_username: 'bob',
      },
    ];

    beforeEach(() => {
      mockFetch.mockResolvedValue({ ok: true, json: async () => mockPosts });
    });

    it('fetches feed', async () => {
      const result = await api.listV1Feed();

      expect(mockFetch).toHaveBeenCalledWith('/api/v1/feed', expect.any(Object));
      expect(result.length).toBe(2);
      expect(result[0].body).toBe('Hello world!');
    });

    it('fetches feed with pagination params', async () => {
      const result = await api.listV1Feed({ page: 2, limit: 10 } as any);

      expect(mockFetch).toHaveBeenCalledWith('/api/v1/feed?page=2&limit=10', expect.any(Object));
    });
  });

  describe('Posts CRUD endpoints', () => {
    const mockPost = {
      id: 1,
      author_id: 1,
      body: 'New post content',
      created_at: new Date().toISOString(),
      post_date: '2024-01-01',
      like_count: 0,
      is_deleted: false,
      author_username: 'testuser',
    };

    beforeEach(() => {
      mockFetch.mockResolvedValue({ ok: true, json: async () => mockPost });
    });

    it('creates a post', async () => {
      const result = await api.createV1Posts({ body: 'New post content' });

      expect(mockFetch).toHaveBeenCalledWith('/api/v1/posts', {
        method: 'POST',
        headers: expect.any(Object),
        body: expect.any(String),
      });
      expect(result.body).toBe('New post content');
    });

    it('fetches a post by ID', async () => {
      const result = await api.getV1PostsById(1);

      expect(mockFetch).toHaveBeenCalledWith('/api/v1/posts/1', expect.any(Object));
      expect(result.id).toBe(1);
    });

    it('deletes a post', async () => {
      const result = await api.removeV1PostsById(1);

      expect(mockFetch).toHaveBeenCalledWith('/api/v1/posts/1', {
        method: 'DELETE',
        headers: expect.any(Object),
      });
      expect(result.is_deleted).toBe(true);
    });
  });

  describe('Like endpoints', () => {
    beforeEach(() => {
      mockFetch.mockResolvedValue({ ok: true, json: async () => ({ like_count: 1 }) });
    });

    it('likes a post', async () => {
      const result = await api.createV1PostsLike(1);

      expect(mockFetch).toHaveBeenCalledWith('/api/v1/posts/1/like', {
        method: 'POST',
        headers: expect.any(Object),
      });
      expect(result.like_count).toBe(1);
    });

    it('unlikes a post', async () => {
      const result = await api.removeV1PostsLike(1);

      expect(mockFetch).toHaveBeenCalledWith('/api/v1/posts/1/unlike', {
        method: 'DELETE',
        headers: expect.any(Object),
      });
      expect(result.like_count).toBe(0);
    });
  });

  describe('User endpoints', () => {
    const mockProfile = {
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: 'Test bio',
      created_at: new Date().toISOString(),
      post_count: 5,
      total_likes_received: 42,
    };

    const mockPosts = [
      {
        id: 1,
        author_id: 1,
        body: 'First post',
        created_at: new Date().toISOString(),
        post_date: '2024-01-01',
        like_count: 5,
        is_deleted: false,
        author_username: 'testuser',
      },
    ];

    beforeEach(() => {
      mockFetch.mockResolvedValue({ ok: true, json: async () => mockProfile });
    });

    it('fetches user profile', async () => {
      const result = await api.getUserProfile('testuser');

      expect(mockFetch).toHaveBeenCalledWith('/api/v1/users/testuser', expect.any(Object));
      expect(result.username).toBe('testuser');
    });

    it('fetches user posts', async () => {
      mockFetch.mockResolvedValue({ ok: true, json: async () => mockPosts });

      const result = await api.getUserPosts('testuser');

      expect(mockFetch).toHaveBeenCalledWith('/api/v1/users/testuser/posts', expect.any(Object));
      expect(result.length).toBe(1);
    });
  });
});

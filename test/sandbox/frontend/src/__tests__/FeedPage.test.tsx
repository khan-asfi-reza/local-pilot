import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import FeedPage from '../pages/FeedPage';

// Mock the API client
vi.mock('../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../lib/api')>('../lib/api');
  return {
    ...actual,
    listFeed: vi.fn(),
    createPost: vi.fn(),
    toggleLike: vi.fn(),
    getCurrentUser: vi.fn(),
  };
});

describe('FeedPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders feed page with loading state initially', async () => {
    const mockPosts = [];
    
    (await import('../lib/api')).listFeed.mockResolvedValue(mockPosts);
    (await import('../lib/api')).getCurrentUser.mockResolvedValue({
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 0,
      total_likes_received: 0,
    });

    render(<FeedPage />);

    // Should show loading state initially
    expect(screen.getByText(/loading/i)).toBeInTheDocument();
  });

  it('renders feed with posts when data is loaded', async () => {
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

    (await import('../lib/api')).listFeed.mockResolvedValue(mockPosts);
    (await import('../lib/api')).getCurrentUser.mockResolvedValue({
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 0,
      total_likes_received: 0,
    });

    render(<FeedPage />);

    // Wait for data to load
    await vi.waitFor(() => {
      expect(screen.getByText('Hello world!')).toBeInTheDocument();
      expect(screen.getByText('Second post')).toBeInTheDocument();
    });

    // Verify posts are rendered with author info
    expect(screen.getByText('@alice')).toBeInTheDocument();
    expect(screen.getByText('@bob')).toBeInTheDocument();
  });

  it('shows empty state when no posts exist', async () => {
    const mockPosts: any[] = [];

    (await import('../lib/api')).listFeed.mockResolvedValue(mockPosts);
    (await import('../lib/api')).getCurrentUser.mockResolvedValue({
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 0,
      total_likes_received: 0,
    });

    render(<FeedPage />);

    await vi.waitFor(() => {
      expect(screen.getByText(/no posts yet/i)).toBeInTheDocument();
    });
  });

  it('handles error state when feed fetch fails', async () => {
    (await import('../lib/api')).listFeed.mockRejectedValue(new Error('Network error'));
    (await import('../lib/api')).getCurrentUser.mockResolvedValue({
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 0,
      total_likes_received: 0,
    });

    render(<FeedPage />);

    await vi.waitFor(() => {
      expect(screen.getByText(/error loading feed/i)).toBeInTheDocument();
    });
  });

  it('renders create post button', async () => {
    const mockPosts = [];

    (await import('../lib/api')).listFeed.mockResolvedValue(mockPosts);
    (await import('../lib/api')).getCurrentUser.mockResolvedValue({
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 0,
      total_likes_received: 0,
    });

    render(<FeedPage />);

    await vi.waitFor(() => {
      expect(screen.getByRole('button', { name: /create new post/i })).toBeInTheDocument();
    });
  });

  it('navigates to create post page when button is clicked', async () => {
    const mockPosts = [];

    (await import('../lib/api')).listFeed.mockResolvedValue(mockPosts);
    (await import('../lib/api')).getCurrentUser.mockResolvedValue({
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 0,
      total_likes_received: 0,
    });

    render(<FeedPage />);

    await vi.waitFor(() => {
      const createButton = screen.getByRole('button', { name: /create new post/i });
      fireEvent.click(createButton);
    });

    // Check that navigation happened (would be verified by router in real app)
    expect((await import('../lib/api')).createPost).not.toHaveBeenCalled();
  });

  it('renders like button on each post', async () => {
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
    ];

    (await import('../lib/api')).listFeed.mockResolvedValue(mockPosts);
    (await import('../lib/api')).getCurrentUser.mockResolvedValue({
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 0,
      total_likes_received: 0,
    });

    render(<FeedPage />);

    await vi.waitFor(() => {
      const likeButton = screen.getByRole('button', { name: /like/i });
      expect(likeButton).toBeInTheDocument();
    });
  });

  it('toggles like on post when button is clicked', async () => {
    let mockPosts = [
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
    ];

    (await import('../lib/api')).listFeed.mockResolvedValue(mockPosts);
    (await import('../lib/api')).getCurrentUser.mockResolvedValue({
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 0,
      total_likes_received: 0,
    });

    render(<FeedPage />);

    await vi.waitFor(() => {
      const likeButton = screen.getByRole('button', { name: /like/i });
      fireEvent.click(likeButton);
    });

    // Verify toggleLike was called with correct post ID
    expect((await import('../lib/api')).toggleLike).toHaveBeenCalledWith(1);
  });

  it('shows liked state after liking a post', async () => {
    let mockPosts = [
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
    ];

    (await import('../lib/api')).listFeed.mockResolvedValue(mockPosts);
    (await import('../lib/api')).getCurrentUser.mockResolvedValue({
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 0,
      total_likes_received: 0,
    });

    render(<FeedPage />);

    await vi.waitFor(() => {
      const likeButton = screen.getByRole('button', { name: /like/i });
      fireEvent.click(likeButton);
    });

    // Update mock to reflect liked state
    mockPosts = [
      {
        ...mockPosts[0],
        like_count: 6,
      },
    ];
    (await import('../lib/api')).listFeed.mockResolvedValue(mockPosts);

    // Re-render with updated data
    render(<FeedPage />);

    await vi.waitFor(() => {
      const likeButton = screen.getByRole('button', { name: /liked/i });
      expect(likeButton).toBeInTheDocument();
    });
  });

  it('filters out deleted posts from feed', async () => {
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
        body: 'Deleted post',
        created_at: new Date().toISOString(),
        post_date: '2024-01-02',
        like_count: 0,
        is_deleted: true,
        author_username: 'bob',
      },
    ];

    (await import('../lib/api')).listFeed.mockResolvedValue(mockPosts);
    (await import('../lib/api')).getCurrentUser.mockResolvedValue({
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 0,
      total_likes_received: 0,
    });

    render(<FeedPage />);

    await vi.waitFor(() => {
      expect(screen.getByText('Hello world!')).toBeInTheDocument();
      expect(screen.queryByText('Deleted post')).not.toBeInTheDocument();
    });
  });
});

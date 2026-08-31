import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { ProfilePage } from '../pages/ProfilePage';

// Mock the API client
vi.mock('../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../lib/api')>('../lib/api');
  return {
    ...actual,
    getUserProfile: vi.fn(),
    getUserPosts: vi.fn(),
    getCurrentUser: vi.fn(),
  };
});

describe('ProfilePage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders profile page with loading state initially', async () => {
    const mockProfile = {
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: 'Test bio',
      created_at: new Date().toISOString(),
      post_count: 5,
      total_likes_received: 42,
    };

    (await import('../lib/api')).getUserProfile.mockResolvedValue(mockProfile);
    (await import('../lib/api')).getUserPosts.mockResolvedValue([]);
    (await import('../lib/api')).getCurrentUser.mockResolvedValue({
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 0,
      total_likes_received: 0,
    });

    render(<ProfilePage username="testuser" />);

    // Should show loading state initially
    expect(screen.getByText(/loading/i)).toBeInTheDocument();
  });

  it('renders profile with user info when data is loaded', async () => {
    const mockProfile = {
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: 'I love coding!',
      created_at: new Date().toISOString(),
      post_count: 5,
      total_likes_received: 42,
    };

    (await import('../lib/api')).getUserProfile.mockResolvedValue(mockProfile);
    (await import('../lib/api')).getUserPosts.mockResolvedValue([]);
    (await import('../lib/api')).getCurrentUser.mockResolvedValue({
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 0,
      total_likes_received: 0,
    });

    render(<ProfilePage username="testuser" />);

    // Wait for data to load
    await vi.waitFor(() => {
      expect(screen.getByText('@testuser')).toBeInTheDocument();
      expect(screen.getByText('I love coding!')).toBeInTheDocument();
    });
  });

  it('shows bio when user has one', async () => {
    const mockProfile = {
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: 'Software engineer passionate about React and TypeScript.',
      created_at: new Date().toISOString(),
      post_count: 5,
      total_likes_received: 42,
    };

    (await import('../lib/api')).getUserProfile.mockResolvedValue(mockProfile);
    (await import('../lib/api')).getUserPosts.mockResolvedValue([]);
    (await import('../lib/api')).getCurrentUser.mockResolvedValue({
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 0,
      total_likes_received: 0,
    });

    render(<ProfilePage username="testuser" />);

    await vi.waitFor(() => {
      expect(screen.getByText('Software engineer passionate about React and TypeScript.')).toBeInTheDocument();
    });
  });

  it('shows empty bio when user has none', async () => {
    const mockProfile = {
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 0,
      total_likes_received: 0,
    };

    (await import('../lib/api')).getUserProfile.mockResolvedValue(mockProfile);
    (await import('../lib/api')).getUserPosts.mockResolvedValue([]);
    (await import('../lib/api')).getCurrentUser.mockResolvedValue({
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 0,
      total_likes_received: 0,
    });

    render(<ProfilePage username="testuser" />);

    await vi.waitFor(() => {
      expect(screen.getByText(/bio/i)).toBeInTheDocument();
    });
  });

  it('renders user stats (post count and likes received)', async () => {
    const mockProfile = {
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 5,
      total_likes_received: 42,
    };

    (await import('../lib/api')).getUserProfile.mockResolvedValue(mockProfile);
    (await import('../lib/api')).getUserPosts.mockResolvedValue([]);
    (await import('../lib/api')).getCurrentUser.mockResolvedValue({
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 0,
      total_likes_received: 0,
    });

    render(<ProfilePage username="testuser" />);

    await vi.waitFor(() => {
      expect(screen.getByText(/5 posts/i)).toBeInTheDocument();
      expect(screen.getByText(/42 likes received/i)).toBeInTheDocument();
    });
  });

  it('renders user posts when they exist', async () => {
    const mockProfile = {
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 2,
      total_likes_received: 10,
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
      {
        id: 2,
        author_id: 1,
        body: 'Second post',
        created_at: new Date().toISOString(),
        post_date: '2024-01-02',
        like_count: 3,
        is_deleted: false,
        author_username: 'testuser',
      },
    ];

    (await import('../lib/api')).getUserProfile.mockResolvedValue(mockProfile);
    (await import('../lib/api')).getUserPosts.mockResolvedValue(mockPosts);
    (await import('../lib/api')).getCurrentUser.mockResolvedValue({
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 0,
      total_likes_received: 0,
    });

    render(<ProfilePage username="testuser" />);

    await vi.waitFor(() => {
      expect(screen.getByText('First post')).toBeInTheDocument();
      expect(screen.getByText('Second post')).toBeInTheDocument();
    });
  });

  it('shows empty state when user has no posts', async () => {
    const mockProfile = {
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 0,
      total_likes_received: 0,
    };

    (await import('../lib/api')).getUserProfile.mockResolvedValue(mockProfile);
    (await import('../lib/api')).getUserPosts.mockResolvedValue([]);
    (await import('../lib/api')).getCurrentUser.mockResolvedValue({
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 0,
      total_likes_received: 0,
    });

    render(<ProfilePage username="testuser" />);

    await vi.waitFor(() => {
      expect(screen.getByText(/no posts yet/i)).toBeInTheDocument();
    });
  });

  it('handles error state when profile fetch fails', async () => {
    (await import('../lib/api')).getUserProfile.mockRejectedValue(new Error('Network error'));
    (await import('../lib/api')).getUserPosts.mockResolvedValue([]);
    (await import('../lib/api')).getCurrentUser.mockResolvedValue({
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 0,
      total_likes_received: 0,
    });

    render(<ProfilePage username="testuser" />);

    await vi.waitFor(() => {
      expect(screen.getByText(/error loading profile/i)).toBeInTheDocument();
    });
  });

  it('handles error state when posts fetch fails', async () => {
    const mockProfile = {
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 0,
      total_likes_received: 0,
    };

    (await import('../lib/api')).getUserProfile.mockResolvedValue(mockProfile);
    (await import('../lib/api')).getUserPosts.mockRejectedValue(new Error('Network error'));
    (await import('../lib/api')).getCurrentUser.mockResolvedValue({
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 0,
      total_likes_received: 0,
    });

    render(<ProfilePage username="testuser" />);

    await vi.waitFor(() => {
      expect(screen.getByText(/error loading posts/i)).toBeInTheDocument();
    });
  });

  it('filters out deleted posts from user profile', async () => {
    const mockProfile = {
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 2,
      total_likes_received: 5,
    };

    const mockPosts = [
      {
        id: 1,
        author_id: 1,
        body: 'Active post',
        created_at: new Date().toISOString(),
        post_date: '2024-01-01',
        like_count: 5,
        is_deleted: false,
        author_username: 'testuser',
      },
      {
        id: 2,
        author_id: 1,
        body: 'Deleted post',
        created_at: new Date().toISOString(),
        post_date: '2024-01-02',
        like_count: 0,
        is_deleted: true,
        author_username: 'testuser',
      },
    ];

    (await import('../lib/api')).getUserProfile.mockResolvedValue(mockProfile);
    (await import('../lib/api')).getUserPosts.mockResolvedValue(mockPosts);
    (await import('../lib/api')).getCurrentUser.mockResolvedValue({
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 0,
      total_likes_received: 0,
    });

    render(<ProfilePage username="testuser" />);

    await vi.waitFor(() => {
      expect(screen.getByText('Active post')).toBeInTheDocument();
      expect(screen.queryByText('Deleted post')).not.toBeInTheDocument();
    });
  });

  it('renders like button on each post', async () => {
    const mockProfile = {
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 1,
      total_likes_received: 5,
    };

    const mockPosts = [
      {
        id: 1,
        author_id: 1,
        body: 'Hello world!',
        created_at: new Date().toISOString(),
        post_date: '2024-01-01',
        like_count: 5,
        is_deleted: false,
        author_username: 'testuser',
      },
    ];

    (await import('../lib/api')).getUserProfile.mockResolvedValue(mockProfile);
    (await import('../lib/api')).getUserPosts.mockResolvedValue(mockPosts);
    (await import('../lib/api')).getCurrentUser.mockResolvedValue({
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 0,
      total_likes_received: 0,
    });

    render(<ProfilePage username="testuser" />);

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
        author_username: 'testuser',
      },
    ];

    (await import('../lib/api')).getUserProfile.mockResolvedValue({
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 1,
      total_likes_received: 5,
    });
    (await import('../lib/api')).getUserPosts.mockResolvedValue(mockPosts);
    (await import('../lib/api')).getCurrentUser.mockResolvedValue({
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 0,
      total_likes_received: 0,
    });

    render(<ProfilePage username="testuser" />);

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
        author_username: 'testuser',
      },
    ];

    (await import('../lib/api')).getUserProfile.mockResolvedValue({
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 1,
      total_likes_received: 5,
    });
    (await import('../lib/api')).getUserPosts.mockResolvedValue(mockPosts);
    (await import('../lib/api')).getCurrentUser.mockResolvedValue({
      id: 1,
      username: 'testuser',
      email: 'test@example.com',
      bio: '',
      created_at: new Date().toISOString(),
      post_count: 0,
      total_likes_received: 0,
    });

    render(<ProfilePage username="testuser" />);

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
    (await import('../lib/api')).getUserPosts.mockResolvedValue(mockPosts);

    // Re-render with updated data
    render(<ProfilePage username="testuser" />);

    await vi.waitFor(() => {
      const likeButton = screen.getByRole('button', { name: /liked/i });
      expect(likeButton).toBeInTheDocument();
    });
  });
});

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import PostCard from '../components/PostCard';

// Mock LikeButton to avoid API calls in tests
vi.mock('../components/LikeButton', () => ({
  default: ({ postId, likeCount }: { postId: number; likeCount: number }) => (
    <button data-testid="like-button" onClick={() => {}}>
      {likeCount} likes
    </button>
  ),
}));

describe('PostCard', () => {
  const mockPost = {
    id: 1,
    author_id: 1,
    body: 'This is a short post that fits within the character limit and should display completely without truncation.',
    created_at: '2024-01-15T10:30:00Z',
    post_date: '2024-01-15',
    like_count: 5,
    is_deleted: false,
    author_username: 'alice',
  };

  const mockLongPost = {
    id: 2,
    author_id: 2,
    body: 'This is a very long post that exceeds the character limit and should be truncated. Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.',
    created_at: '2024-01-15T11:00:00Z',
    post_date: '2024-01-15',
    like_count: 10,
    is_deleted: false,
    author_username: 'bob',
  };

  const mockDeletedPost = {
    ...mockPost,
    is_deleted: true,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders post with author info and date', () => {
    render(<PostCard post={mockPost} />);

    expect(screen.getByText('alice')).toBeInTheDocument();
    expect(screen.getByText(/Jan 15, 2024/)).toBeInTheDocument();
    expect(screen.getByText('This is a short post that fits within the character limit and should display completely without truncation.')).toBeInTheDocument();
  });

  it('renders truncated text for long posts', () => {
    render(<PostCard post={mockLongPost} />);

    const displayedText = screen.getByText(/This is a very long post that exceeds the character limit and should be truncated\.\.\./);
    expect(displayedText).toBeInTheDocument();
  });

  it('shows "Read more" button for truncated posts', () => {
    render(<PostCard post={mockLongPost} />);

    const readMoreButton = screen.getByText('Read more');
    expect(readMoreButton).toBeInTheDocument();

    fireEvent.click(readMoreButton);

    // Should show full text and hide "Read more"
    expect(screen.getByText(/This is a very long post that exceeds the character limit and should be truncated\. Lorem ipsum dolor sit amet/)).toBeInTheDocument();
    expect(screen.queryByText('Read more')).not.toBeInTheDocument();
  });

  it('shows "Show less" button after expanding', () => {
    render(<PostCard post={mockLongPost} />);

    const readMoreButton = screen.getByText('Read more');
    fireEvent.click(readMoreButton);

    const showLessButton = screen.getByText('Show less');
    expect(showLessButton).toBeInTheDocument();

    fireEvent.click(showLessButton);

    // Should truncate again
    expect(screen.queryByText(/Lorem ipsum dolor sit amet/)).not.toBeInTheDocument();
  });

  it('renders deleted post with deletion notice', () => {
    render(<PostCard post={mockDeletedPost} />);

    expect(screen.getByText('This is a short post that fits within the character limit and should display completely without truncation.')).toBeInTheDocument();
    expect(screen.getByText(/This post has been deleted/)).toBeInTheDocument();
  });

  it('renders like button with correct count', () => {
    render(<PostCard post={mockPost} />);

    const likeButton = screen.getByTestId('like-button');
    expect(likeButton).toHaveTextContent('5 likes');
  });

  it('renders avatar with first letter of username', () => {
    render(<PostCard post={mockPost} />);

    const avatar = screen.getByText('A');
    expect(avatar).toBeInTheDocument();
  });

  it('handles empty body gracefully', () => {
    const emptyPost = {
      ...mockPost,
      body: '',
    };

    render(<PostCard post={emptyPost} />);

    expect(screen.getByText('alice')).toBeInTheDocument();
    expect(screen.queryByText(/This is a short post/)).not.toBeInTheDocument();
  });

  it('handles very long usernames', () => {
    const longUsernamePost = {
      ...mockPost,
      author_username: 'verylongusername123456789012345678901234567890',
    };

    render(<PostCard post={longUsernamePost} />);

    expect(screen.getByText('V')).toBeInTheDocument(); // First letter capitalized
  });

  it('renders correctly with zero likes', () => {
    const noLikesPost = {
      ...mockPost,
      like_count: 0,
    };

    render(<PostCard post={noLikesPost} />);

    expect(screen.getByText('0 likes')).toBeInTheDocument();
  });

  it('renders correctly with negative like count (edge case)', () => {
    const negativeLikesPost = {
      ...mockPost,
      like_count: -1,
    };

    render(<PostCard post={negativeLikesPost} />);

    expect(screen.getByText('-1 likes')).toBeInTheDocument();
  });

  it('handles posts with special characters in body', () => {
    const specialCharsPost = {
      ...mockPost,
      body: 'Hello <world> & "quotes" and \'apostrophes\'!',
    };

    render(<PostCard post={specialCharsPost} />);

    expect(screen.getByText('Hello <world> & "quotes" and \'apostrophes\'!')).toBeInTheDocument();
  });

  it('handles posts with newlines', () => {
    const multilinePost = {
      ...mockPost,
      body: 'Line one\n\nLine two\n\nLine three',
    };

    render(<PostCard post={multilinePost} />);

    expect(screen.getByText('Line one')).toBeInTheDocument();
    expect(screen.getByText('Line two')).toBeInTheDocument();
    expect(screen.getByText('Line three')).toBeInTheDocument();
  });
});

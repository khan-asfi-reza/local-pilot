import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import LikeButton from '../components/LikeButton';

// Mock the API client
vi.mock('../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../lib/api')>('../lib/api');
  return {
    ...actual,
    createV1PostsLike: vi.fn(),
    removeV1PostsLike: vi.fn(),
  };
});

describe('LikeButton', () => {
  const mockCreateLike = vi.mocked((await import('../lib/api')).createV1PostsLike);
  const mockRemoveLike = vi.mocked((await import('../lib/api')).removeV1PostsLike);

  beforeEach(() => {
    vi.clearAllMocks();
    mockCreateLike.mockResolvedValue({ like_count: 1 });
    mockRemoveLike.mockResolvedValue({ like_count: 0 });
  });

  it('renders with initial like count', () => {
    render(<LikeButton postId={1} likeCount={5} />);

    expect(screen.getByText('5')).toBeInTheDocument();
    const button = screen.getByRole('button');
    expect(button).toHaveTextContent('5');
  });

  it('shows liked state when user has liked the post', async () => {
    mockCreateLike.mockResolvedValue({ like_count: 10 });

    render(<LikeButton postId={1} likeCount={5} />);

    const button = screen.getByRole('button');
    fireEvent.click(button);

    await waitFor(() => {
      expect(mockCreateLike).toHaveBeenCalledWith(1);
    });

    // Should show updated count and liked state
    expect(screen.getByText('6')).toBeInTheDocument();
    expect(button).toHaveAttribute('aria-label', 'Liked 6 times');
  });

  it('toggles like when clicked again', async () => {
    mockCreateLike.mockResolvedValue({ like_count: 10 });
    mockRemoveLike.mockResolvedValue({ like_count: 9 });

    render(<LikeButton postId={1} likeCount={5} />);

    const button = screen.getByRole('button');
    
    // First click - like
    fireEvent.click(button);
    await waitFor(() => {
      expect(mockCreateLike).toHaveBeenCalledWith(1);
    });
    expect(screen.getByText('6')).toBeInTheDocument();

    // Second click - unlike
    fireEvent.click(button);
    await waitFor(() => {
      expect(mockRemoveLike).toHaveBeenCalledWith(1);
    });
    expect(screen.getByText('5')).toBeInTheDocument();
  });

  it('shows loading state during API call', async () => {
    mockCreateLike.mockImplementation(async () => {
      await new Promise(resolve => setTimeout(resolve, 10));
      return { like_count: 10 };
    });

    render(<LikeButton postId={1} likeCount={5} />);

    const button = screen.getByRole('button');
    expect(button).not.toBeDisabled();

    fireEvent.click(button);

    await waitFor(() => {
      expect(button).toBeDisabled();
    });

    // After API completes, should be enabled again
    await waitFor(() => {
      expect(button).not.toBeDisabled();
    });
  });

  it('shows error message when like fails', async () => {
    mockCreateLike.mockRejectedValue(new Error('Rate limit exceeded'));

    render(<LikeButton postId={1} likeCount={5} />);

    const button = screen.getByRole('button');
    fireEvent.click(button);

    await waitFor(() => {
      expect(screen.getByText('Rate limit exceeded')).toBeInTheDocument();
    });
  });

  it('shows generic error message for unknown errors', async () => {
    mockCreateLike.mockRejectedValue(new Error('Something went wrong'));

    render(<LikeButton postId={1} likeCount={5} />);

    const button = screen.getByRole('button');
    fireEvent.click(button);

    await waitFor(() => {
      expect(screen.getByText('Failed to like post')).toBeInTheDocument();
    });
  });

  it('shows liked heart icon when liked', async () => {
    mockCreateLike.mockResolvedValue({ like_count: 10 });

    render(<LikeButton postId={1} likeCount={5} />);

    const button = screen.getByRole('button');
    fireEvent.click(button);

    await waitFor(() => {
      // Should have filled heart icon (path with fill-current)
      expect(screen.getByText(/fill-current/)).toBeInTheDocument();
    });
  });

  it('shows outline heart icon when not liked', () => {
    render(<LikeButton postId={1} likeCount={5} />);

    const button = screen.getByRole('button');
    // Should have outline heart icon (path with fill-none)
    expect(screen.getByText(/fill-none/)).toBeInTheDocument();
  });

  it('shows disabled state when loading', async () => {
    mockCreateLike.mockImplementation(async () => {
      await new Promise(resolve => setTimeout(resolve, 10));
      return { like_count: 10 };
    });

    render(<LikeButton postId={1} likeCount={5} />);

    const button = screen.getByRole('button');
    fireEvent.click(button);

    await waitFor(() => {
      expect(button).toHaveAttribute('aria-disabled', 'true');
    });
  });

  it('shows opacity reduced when disabled', async () => {
    mockCreateLike.mockImplementation(async () => {
      await new Promise(resolve => setTimeout(resolve, 10));
      return { like_count: 10 };
    });

    render(<LikeButton postId={1} likeCount={5} />);

    const button = screen.getByRole('button');
    fireEvent.click(button);

    await waitFor(() => {
      expect(button).toHaveAttribute('aria-disabled', 'true');
    });
  });

  it('handles zero likes correctly', async () => {
    mockCreateLike.mockResolvedValue({ like_count: 0 });

    render(<LikeButton postId={1} likeCount={0} />);

    const button = screen.getByRole('button');
    expect(button).toHaveTextContent('0');

    fireEvent.click(button);

    await waitFor(() => {
      expect(mockCreateLike).toHaveBeenCalledWith(1);
    });

    expect(screen.getByText('1')).toBeInTheDocument();
  });

  it('handles large like counts', () => {
    render(<LikeButton postId={1} likeCount={999999} />);

    const button = screen.getByRole('button');
    expect(button).toHaveTextContent('999999');
  });

  it('shows correct aria-label for liked state', async () => {
    mockCreateLike.mockResolvedValue({ like_count: 10 });

    render(<LikeButton postId={1} likeCount={5} />);

    const button = screen.getByRole('button');
    fireEvent.click(button);

    await waitFor(() => {
      expect(button).toHaveAttribute('aria-label', 'Liked 6 times');
    });
  });

  it('shows correct aria-label for not liked state', () => {
    render(<LikeButton postId={1} likeCount={5} />);

    const button = screen.getByRole('button');
    expect(button).toHaveAttribute('aria-label', 'Like 5 times');
  });

  it('updates aria-label after toggle', async () => {
    mockCreateLike.mockResolvedValue({ like_count: 10 });
    mockRemoveLike.mockResolvedValue({ like_count: 9 });

    render(<LikeButton postId={1} likeCount={5} />);

    const button = screen.getByRole('button');
    
    // Initially not liked
    expect(button).toHaveAttribute('aria-label', 'Like 5 times');

    // Like it
    fireEvent.click(button);
    await waitFor(() => {
      expect(button).toHaveAttribute('aria-label', 'Liked 6 times');
    });

    // Unlike it
    fireEvent.click(button);
    await waitFor(() => {
      expect(button).toHaveAttribute('aria-label', 'Like 5 times');
    });
  });

  it('handles network errors gracefully', async () => {
    mockCreateLike.mockRejectedValue(new TypeError('Network error'));

    render(<LikeButton postId={1} likeCount={5} />);

    const button = screen.getByRole('button');
    fireEvent.click(button);

    await waitFor(() => {
      expect(screen.getByText('Failed to like post')).toBeInTheDocument();
    });
  });

  it('maintains state after multiple rapid clicks', async () => {
    mockCreateLike.mockResolvedValue({ like_count: 10 });

    render(<LikeButton postId={1} likeCount={5} />);

    const button = screen.getByRole('button');
    
    // Click rapidly - only last click should count
    fireEvent.click(button);
    fireEvent.click(button);
    fireEvent.click(button);

    await waitFor(() => {
      expect(mockCreateLike).toHaveBeenCalledTimes(1);
    });

    expect(screen.getByText('6')).toBeInTheDocument();
  });

  it('shows pink background when liked', async () => {
    mockCreateLike.mockResolvedValue({ like_count: 10 });

    render(<LikeButton postId={1} likeCount={5} />);

    const button = screen.getByRole('button');
    expect(button).toHaveAttribute('class', expect.stringContaining('bg-gray-100'));

    fireEvent.click(button);

    await waitFor(() => {
      expect(button).toHaveAttribute('class', expect.stringContaining('bg-pink-500'));
    });
  });

  it('shows gray background when not liked', () => {
    render(<LikeButton postId={1} likeCount={5} />);

    const button = screen.getByRole('button');
    expect(button).toHaveAttribute('class', expect.stringContaining('bg-gray-100'));
  });

  it('shows white text when liked', async () => {
    mockCreateLike.mockResolvedValue({ like_count: 10 });

    render(<LikeButton postId={1} likeCount={5} />);

    const button = screen.getByRole('button');
    expect(button).toHaveAttribute('class', expect.stringContaining('text-gray-700'));

    fireEvent.click(button);

    await waitFor(() => {
      expect(button).toHaveAttribute('class', expect.stringContaining('text-white'));
    });
  });

  it('shows gray text when not liked', () => {
    render(<LikeButton postId={1} likeCount={5} />);

    const button = screen.getByRole('button');
    expect(button).toHaveAttribute('class', expect.stringContaining('text-gray-700'));
  });

  it('shows hover state for not liked', () => {
    render(<LikeButton postId={1} likeCount={5} />);

    const button = screen.getByRole('button');
    
    fireEvent.mouseEnter(button);
    
    expect(button).toHaveAttribute('class', expect.stringContaining('hover:bg-gray-200'));
  });

  it('shows hover state for liked', async () => {
    mockCreateLike.mockResolvedValue({ like_count: 10 });

    render(<LikeButton postId={1} likeCount={5} />);

    const button = screen.getByRole('button');
    fireEvent.click(button);

    await waitFor(() => {
      expect(button).toHaveAttribute('class', expect.stringContaining('hover:bg-pink-600'));
    });
  });

  it('shows focus ring for accessibility', () => {
    render(<LikeButton postId={1} likeCount={5} />);

    const button = screen.getByRole('button');
    expect(button).toHaveAttribute('tabindex', '0');
  });

  it('focuses correctly when tabbed to', () => {
    render(<LikeButton postId={1} likeCount={5} />);

    const button = screen.getByRole('button');
    button.focus();

    expect(document.activeElement).toBe(button);
  });

  it('handles very large like counts without overflow', () => {
    render(<LikeButton postId={1} likeCount={999999999} />);

    const button = screen.getByRole('button');
    expect(button).toHaveTextContent('999999999');
  });

  it('handles like count of 1', () => {
    render(<LikeButton postId={1} likeCount={1} />);

    const button = screen.getByRole('button');
    expect(button).toHaveTextContent('1');
  });
});

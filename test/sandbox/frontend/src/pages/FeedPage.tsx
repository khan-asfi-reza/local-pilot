import { useEffect, useState } from 'react';
import { listV1Feed } from '../lib/api';
import PostCard from '../components/PostCard';

function FeedPage() {
  const [posts, setPosts] = useState<Array<{
    id: number;
    author_id: number;
    body: string;
    created_at: string;
    post_date: string;
    like_count: number;
    is_deleted: boolean;
    author_username: string;
  }>>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    async function loadFeed() {
      try {
        const data = await listV1Feed();
        setPosts(data);
      } catch (err: unknown) {
        if (err instanceof Error) {
          setError(err.message);
        } else {
          setError('Unknown error loading feed');
        }
      } finally {
        setLoading(false);
      }
    }

    loadFeed();
  }, []);

  if (loading) {
    return (
      <div className="min-h-screen bg-gray-50 p-6">
        <div className="max-w-2xl mx-auto">
          <div className="animate-pulse space-y-4">
            <div className="h-8 bg-gray-200 rounded w-1/3"></div>
            <div className="h-40 bg-gray-200 rounded"></div>
            <div className="h-40 bg-gray-200 rounded"></div>
          </div>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="min-h-screen bg-gray-50 p-6 flex items-center justify-center">
        <div className="max-w-md text-center text-red-600">
          <h1 className="text-2xl font-bold mb-4">Error loading feed</h1>
          <p>{error}</p>
        </div>
      </div>
    );
  }

  if (posts.length === 0) {
    return (
      <div className="min-h-screen bg-gray-50 p-6 flex items-center justify-center">
        <div className="text-center text-gray-600">
          <h1 className="text-2xl font-bold mb-4">No posts yet</h1>
          <p className="mb-4">Be the first to share something!</p>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-50 p-6">
      <div className="max-w-2xl mx-auto">
        <h1 className="text-3xl font-bold text-gray-900 mb-8">Feed</h1>
        <div className="space-y-4">
          {posts.map((post) => (
            <PostCard key={post.id} post={post} />
          ))}
        </div>
      </div>
    </div>
  );
}

export default FeedPage;

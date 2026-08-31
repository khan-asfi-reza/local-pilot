import { useEffect, useState } from 'react';
import { useParams } from 'react-router-dom';
import { getV1UsersByUsername, listV1UsersPosts } from '../lib/api';
import type { getV1UsersByUsernameResult, listV1UsersPostsResult } from '../lib/api';
import { UserStats } from '../components/UserStats';
import { UserPostsList } from '../components/UserPostsList';

interface ProfilePageProps {
  username?: string;
}

export function ProfilePage({ username: propUsername }: ProfilePageProps = {}) {
  const params = useParams();
  const username = propUsername ?? params.username ?? '';
  const [user, setUser] = useState<getV1UsersByUsernameResult | null>(null);
  const [posts, setPosts] = useState<listV1UsersPostsResult[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    async function loadProfile() {
      try {
        const profile = await getV1UsersByUsername(username);
        setUser(profile);

        const userPosts = await listV1UsersPosts(username);
        setPosts(userPosts);
      } catch (err) {
        if ((err as { status?: number }).status === 404) {
          setError('User not found.');
        } else {
          setError('Failed to load profile.');
        }
      } finally {
        setLoading(false);
      }
    }

    loadProfile();
  }, [username]);

  if (loading) {
    return (
      <div className="max-w-2xl mx-auto p-6">
        <div className="animate-pulse space-y-4">
          <div className="h-8 bg-gray-200 rounded w-1/3"></div>
          <div className="h-4 bg-gray-200 rounded w-1/4"></div>
          <div className="h-20 bg-gray-200 rounded"></div>
          <div className="grid grid-cols-3 gap-4">
            <div className="h-24 bg-gray-200 rounded"></div>
            <div className="h-24 bg-gray-200 rounded"></div>
            <div className="h-24 bg-gray-200 rounded"></div>
          </div>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="max-w-2xl mx-auto p-6 text-center">
        <div className="bg-red-50 border border-red-200 rounded-xl p-6">
          <p className="text-red-800">{error}</p>
        </div>
      </div>
    );
  }

  if (!user) {
    return null;
  }

  return (
    <div className="max-w-2xl mx-auto p-6 space-y-6">
      <header className="mb-8">
        <nav className="flex items-center text-sm text-gray-500 mb-4">
          <span>Profile</span>
          <span className="mx-2">/</span>
          <span className="text-gray-900 font-medium">@{user.username}</span>
        </nav>
        <h1 className="text-3xl font-bold text-gray-900">@{user.username}</h1>
      </header>

      <UserStats user={user} />

      <section>
        <h2 className="text-xl font-semibold text-gray-900 mb-4">Posts</h2>
        <UserPostsList posts={posts} />
      </section>
    </div>
  );
}

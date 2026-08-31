import type { getV1UsersByUsernameResult } from '../lib/api'

interface UserStatsProps {
  user: getV1UsersByUsernameResult
}

export function UserStats({ user }: UserStatsProps) {
  return (
    <div className="bg-white rounded-xl shadow-sm border p-6">
      <h2 className="text-2xl font-bold text-gray-900 mb-4">@{user.username}</h2>
      <p className="text-gray-600 mb-6">{user.bio || 'No bio available.'}</p>

      <div className="grid grid-cols-3 gap-4">
        <div className="bg-blue-50 rounded-lg p-4 text-center">
          <div className="text-3xl font-bold text-blue-600">{user.post_count}</div>
          <div className="text-sm text-gray-600 mt-1">Posts</div>
        </div>
        <div className="bg-green-50 rounded-lg p-4 text-center">
          <div className="text-3xl font-bold text-green-600">{user.total_likes_received}</div>
          <div className="text-sm text-gray-600 mt-1">Total Likes</div>
        </div>
        <div className="bg-purple-50 rounded-lg p-4 text-center">
          <div className="text-sm font-medium text-purple-700">{new Date(user.created_at).toLocaleDateString()}</div>
          <div className="text-xs text-gray-600 mt-1">Joined</div>
        </div>
      </div>
    </div>
  )
}

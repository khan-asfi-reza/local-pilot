import type { listV1UsersPostsResult } from '../lib/api'

interface UserPostsListProps {
  posts: listV1UsersPostsResult[]
}

export function UserPostsList({ posts }: UserPostsListProps) {
  if (posts.length === 0) {
    return (
      <div className="bg-white rounded-xl shadow-sm border p-8 text-center">
        <p className="text-gray-500">No posts yet.</p>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {posts.map((post) => (
        <article key={post.id} className="bg-white rounded-xl shadow-sm border p-6 hover:shadow-md transition-shadow">
          <div className="flex items-start justify-between mb-3">
            <time className="text-sm text-gray-500">{new Date(post.created_at).toLocaleDateString()}</time>
            <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-gray-100 text-gray-800">
              {post.like_count} likes
            </span>
          </div>
          <p className="text-gray-800 whitespace-pre-wrap">{post.body}</p>
        </article>
      ))}
    </div>
  )
}

import { useState } from 'react'
import LikeButton from './LikeButton'

interface PostCardProps {
  post: {
    id: number
    author_id: number
    body: string
    created_at: string
    post_date: string
    like_count: number
    is_deleted: boolean
    author_username: string
  }
}

function PostCard({ post }: PostCardProps) {
  const [showFullBody, setShowFullBody] = useState(false)

  function formatDate(dateString: string): string {
    return new Date(dateString).toLocaleDateString('en-US', {
      month: 'short',
      day: 'numeric',
      year: 'numeric',
    })
  }

  function truncateText(text: string, maxLength: number): string {
    if (text.length <= maxLength) {
      return text
    }
    return text.slice(0, maxLength - 1) + '...'
  }

  const displayBody = showFullBody ? post.body : truncateText(post.body, 280)
  const isTruncated = post.body.length > 280

  return (
    <div className="bg-white rounded-xl shadow-sm border p-6">
      <div className="flex items-start justify-between mb-3">
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 bg-blue-100 rounded-full flex items-center justify-center text-blue-600 font-bold">
            {post.author_username.charAt(0).toUpperCase()}
          </div>
          <div>
            <div className="font-semibold text-gray-900">{post.author_username}</div>
            <div className="text-sm text-gray-500">
              {formatDate(post.created_at)} · {post.post_date}
            </div>
          </div>
        </div>
      </div>

      <div className="text-gray-800 leading-relaxed whitespace-pre-wrap">
        {displayBody}
        {isTruncated && (
          <button
            onClick={() => setShowFullBody(!showFullBody)}
            className="text-blue-600 hover:text-blue-700 text-sm mt-2"
          >
            {showFullBody ? 'Show less' : 'Read more'}
          </button>
        )}
      </div>

      <div className="flex items-center justify-between mt-4 pt-4 border-t border-gray-100">
        <LikeButton postId={post.id} likeCount={post.like_count} />
        {post.is_deleted && (
          <span className="text-xs text-gray-400 italic">This post has been deleted</span>
        )}
      </div>
    </div>
  )
}

export default PostCard

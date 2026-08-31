import { useState } from 'react'
import { createV1PostsLike, removeV1PostsLike } from '../lib/api'

interface LikeButtonProps {
  postId: number
  likeCount: number
}

function LikeButton({ postId, likeCount }: LikeButtonProps) {
  const [isLiked, setIsLiked] = useState(false)
  const [likedCount, setLikedCount] = useState(likeCount)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function handleLike() {
    if (isLiked) {
      await unlikePost()
    } else {
      await likePost()
    }
  }

  async function likePost() {
    setLoading(true)
    setError(null)
    try {
      const result = await createV1PostsLike(postId)
      setLikedCount(result.like_count)
      setIsLiked(true)
    } catch (err: unknown) {
      if (err instanceof Error) {
        setError(err.message)
      } else {
        setError('Failed to like post')
      }
    } finally {
      setLoading(false)
    }
  }

  async function unlikePost() {
    setLoading(true)
    setError(null)
    try {
      const result = await removeV1PostsLike(postId)
      setLikedCount(result.like_count)
      setIsLiked(false)
    } catch (err: unknown) {
      if (err instanceof Error) {
        setError(err.message)
      } else {
        setError('Failed to unlike post')
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="flex items-center gap-3">
      <button
        onClick={handleLike}
        disabled={loading}
        className={`flex items-center gap-2 px-4 py-2 rounded-full font-medium transition-all ${
          isLiked
            ? 'bg-pink-500 text-white hover:bg-pink-600 focus:ring-4 focus:ring-pink-300'
            : 'bg-gray-100 text-gray-700 hover:bg-gray-200 focus:ring-4 focus:ring-gray-300'
        } disabled:opacity-50 disabled:cursor-not-allowed`}
      >
        <svg
          className={`w-5 h-5 ${isLiked ? 'fill-current' : 'fill-none'}`}
          viewBox="0 0 24 24"
          stroke="currentColor"
          strokeWidth={isLiked ? 0 : 2}
        >
          {isLiked ? (
            <path d="M12 21.35l-1.45-1.32C5.4 15.36 2 12.28 2 8.5 2 5.42 4.42 3 7.5 3c1.74 0 3.41.81 4.5 2.09C13.09 3.81 14.76 3 16.5 3 19.58 3 22 5.42 22 8.5c0 3.78-3.4 6.86-8.55 11.54L12 21.35z" />
          ) : (
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M4.318 6.318a4.5 4.5 0 000 6.364L12 20.364l7.682-7.682a4.5 4.5 0 00-6.364-6.364L12 7.636l-1.318-1.318a4.5 4.5 0 00-6.364 0z"
            />
          )}
        </svg>
        <span>{likedCount}</span>
      </button>
      {error && (
        <span className="text-sm text-red-600">{error}</span>
      )}
    </div>
  )
}

export default LikeButton

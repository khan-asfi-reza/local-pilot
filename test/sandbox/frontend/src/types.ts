export interface User {
  id: number;
  username: string;
  email: string;
  bio?: string;
  post_count: number;
  total_likes_received: number;
  created_at: string;
}

export interface Post {
  id: number;
  user_id: number;
  body: string;
  created_at: string;
  like_count: number;
}

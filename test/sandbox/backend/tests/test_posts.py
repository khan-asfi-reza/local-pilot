"""Tests for post CRUD and like endpoints."""

import pytest
from django.contrib.auth import get_user_model
from django.urls import reverse
from rest_framework import status


User = get_user_model()


class TestFeed:
    """Test feed endpoint."""

    def test_feed_empty(self, client):
        """Test feed with no posts."""
        response = client.get(reverse("posts:feed"), format=True)
        
        assert response.status_code == status.HTTP_200_OK
        assert response.data == []

    def test_feed_with_posts(self, client):
        """Test feed with multiple posts from different users."""
        # Create users and posts
        user1 = User.objects.create_user(
            username="alice",
            email="alice@example.com",
            password="pass123"
        )
        user2 = User.objects.create_user(
            username="bob",
            email="bob@example.com",
            password="pass456"
        )
        
        from core.models import Post
        
        # Create posts with different dates and like counts
        post1 = Post.objects.create(
            author=user1,
            body="Hello world!",
            post_date="2024-01-01",
            like_count=5
        )
        post2 = Post.objects.create(
            author=user2,
            body="Second post",
            post_date="2024-01-01",
            like_count=3
        )
        post3 = Post.objects.create(
            author=user1,
            body="Third post",
            post_date="2024-01-02",
            like_count=10
        )
        
        response = client.get(reverse("posts:feed"), format=True)
        
        assert response.status_code == status.HTTP_200_OK
        posts = response.data
        
        # Verify all posts are present
        post_ids = {p["id"] for p in posts}
        assert post_ids == {post1.id, post2.id, post3.id}

    def test_feed_ordering(self, client):
        """Test feed is ordered by date then like count."""
        user1 = User.objects.create_user(
            username="alice",
            email="alice@example.com",
            password="pass123"
        )
        
        from core.models import Post
        
        # Create posts on same day with different like counts
        post_high_likes = Post.objects.create(
            author=user1,
            body="High likes",
            post_date="2024-01-01",
            like_count=100
        )
        post_low_likes = Post.objects.create(
            author=user1,
            body="Low likes",
            post_date="2024-01-01",
            like_count=5
        )
        
        response = client.get(reverse("posts:feed"), format=True)
        posts = response.data
        
        # Posts on same day should be ordered by like count (descending)
        assert posts[0]["id"] == post_high_likes.id
        assert posts[1]["id"] == post_low_likes.id


class TestCreatePost:
    """Test creating posts."""

    def test_create_post_success(self, client):
        """Test successful post creation."""
        # Create user and login
        user = User.objects.create_user(
            username="testuser",
            email="test@example.com",
            password="securepass123"
        )
        
        response = client.post(reverse("posts:create"), {
            "body": "New post content"
        }, format=True, **{"HTTP_AUTHORIZATION": f"Bearer testtoken"})
        
        assert response.status_code == status.HTTP_201_CREATED
        assert response.data["body"] == "New post content"
        assert "id" in response.data

    def test_create_post_without_auth(self, client):
        """Test creating post fails without authentication."""
        response = client.post(reverse("posts:create"), {
            "body": "New post content"
        }, format=True)
        
        assert response.status_code == status.HTTP_401_UNAUTHORIZED

    def test_create_post_empty_body(self, client):
        """Test creating post with empty body fails."""
        user = User.objects.create_user(
            username="testuser",
            email="test@example.com",
            password="securepass123"
        )
        
        response = client.post(reverse("posts:create"), {
            "body": ""
        }, format=True, **{"HTTP_AUTHORIZATION": f"Bearer testtoken"})
        
        assert response.status_code == status.HTTP_400_BAD_REQUEST


class TestGetPost:
    """Test getting single post by ID."""

    def test_get_post_success(self, client):
        """Test getting existing post."""
        user = User.objects.create_user(
            username="testuser",
            email="test@example.com",
            password="securepass123"
        )
        
        from core.models import Post
        
        post = Post.objects.create(
            author=user,
            body="Test post content",
            post_date="2024-01-01",
            like_count=5
        )
        
        response = client.get(f"{reverse('posts:post', kwargs={'pk': post.id})}", format=True)
        
        assert response.status_code == status.HTTP_200_OK
        assert response.data["id"] == post.id
        assert response.data["body"] == "Test post content"

    def test_get_post_nonexistent(self, client):
        """Test getting nonexistent post returns 404."""
        response = client.get(reverse("posts:post", kwargs={"pk": 9999}), format=True)
        
        assert response.status_code == status.HTTP_404_NOT_FOUND


class TestDeletePost:
    """Test soft delete of posts."""

    def test_delete_post_success(self, client):
        """Test successful post deletion by author."""
        user = User.objects.create_user(
            username="testuser",
            email="test@example.com",
            password="securepass123"
        )
        
        from core.models import Post
        
        post = Post.objects.create(
            author=user,
            body="To be deleted",
            post_date="2024-01-01",
            like_count=5
        )
        
        response = client.delete(f"{reverse('posts:post', kwargs={'pk': post.id})}", format=True)
        
        assert response.status_code == status.HTTP_200_OK
        assert response.data["is_deleted"] is True

    def test_delete_post_nonexistent(self, client):
        """Test deleting nonexistent post returns 404."""
        response = client.delete(reverse("posts:post", kwargs={"pk": 9999}), format=True)
        
        assert response.status_code == status.HTTP_404_NOT_FOUND

    def test_delete_post_not_author(self, client):
        """Test deleting post fails if not the author."""
        user1 = User.objects.create_user(
            username="alice",
            email="alice@example.com",
            password="pass123"
        )
        user2 = User.objects.create_user(
            username="bob",
            email="bob@example.com",
            password="pass456"
        )
        
        from core.models import Post
        
        post = Post.objects.create(
            author=user1,
            body="Alice's post",
            post_date="2024-01-01",
            like_count=5
        )
        
        # Bob tries to delete Alice's post
        response = client.delete(f"{reverse('posts:post', kwargs={'pk': post.id})}", format=True)
        
        assert response.status_code == status.HTTP_403_FORBIDDEN


class TestLikePost:
    """Test liking posts."""

    def test_like_post_success(self, client):
        """Test successful like on post."""
        user = User.objects.create_user(
            username="testuser",
            email="test@example.com",
            password="securepass123"
        )
        
        from core.models import Post
        
        post = Post.objects.create(
            author=user,
            body="Likeable post",
            post_date="2024-01-01",
            like_count=0
        )
        
        response = client.post(f"{reverse('posts:like', kwargs={'pk': post.id})}", format=True)
        
        assert response.status_code == status.HTTP_200_OK
        assert response.data["like_count"] == 1

    def test_like_idempotent(self, client):
        """Test liking same post twice only increments once."""
        user = User.objects.create_user(
            username="testuser",
            email="test@example.com",
            password="securepass123"
        )
        
        from core.models import Post
        
        post = Post.objects.create(
            author=user,
            body="Likeable post",
            post_date="2024-01-01",
            like_count=0
        )
        
        # Like twice
        response1 = client.post(f"{reverse('posts:like', kwargs={'pk': post.id})}", format=True)
        response2 = client.post(f"{reverse('posts:like', kwargs={'pk': post.id})}", format=True)
        
        assert response1.status_code == status.HTTP_200_OK
        assert response2.status_code == status.HTTP_200_OK
        
        # Should only be 1 like total
        assert response1.data["like_count"] == 1
        assert response2.data["like_count"] == 1

    def test_like_without_auth(self, client):
        """Test liking post fails without authentication."""
        from core.models import Post
        
        user = User.objects.create_user(
            username="testuser",
            email="test@example.com",
            password="securepass123"
        )
        
        post = Post.objects.create(
            author=user,
            body="Likeable post",
            post_date="2024-01-01",
            like_count=0
        )
        
        response = client.post(f"{reverse('posts:like', kwargs={'pk': post.id})}", format=True)
        
        assert response.status_code == status.HTTP_401_UNAUTHORIZED


class TestUnlikePost:
    """Test unliking posts."""

    def test_unlike_post_success(self, client):
        """Test successful unlike on post."""
        user = User.objects.create_user(
            username="testuser",
            email="test@example.com",
            password="securepass123"
        )
        
        from core.models import Post
        
        post = Post.objects.create(
            author=user,
            body="Unlikable post",
            post_date="2024-01-01",
            like_count=5
        )
        
        # Like first
        client.post(f"{reverse('posts:like', kwargs={'pk': post.id})}", format=True)
        
        # Then unlike
        response = client.delete(f"{reverse('posts:unlike', kwargs={'pk': post.id})}", format=True)
        
        assert response.status_code == status.HTTP_200_OK
        assert response.data["like_count"] == 4

    def test_unlike_idempotent(self, client):
        """Test unliking when not liked has no effect."""
        user = User.objects.create_user(
            username="testuser",
            email="test@example.com",
            password="securepass123"
        )
        
        from core.models import Post
        
        post = Post.objects.create(
            author=user,
            body="Unlikable post",
            post_date="2024-01-01",
            like_count=5
        )
        
        # Unlike when not liked
        response = client.delete(f"{reverse('posts:unlike', kwargs={'pk': post.id})}", format=True)
        
        assert response.status_code == status.HTTP_200_OK
        assert response.data["like_count"] == 5

    def test_unlike_without_auth(self, client):
        """Test unliking post fails without authentication."""
        from core.models import Post
        
        user = User.objects.create_user(
            username="testuser",
            email="test@example.com",
            password="securepass123"
        )
        
        post = Post.objects.create(
            author=user,
            body="Unlikable post",
            post_date="2024-01-01",
            like_count=5
        )
        
        response = client.delete(f"{reverse('posts:unlike', kwargs={'pk': post.id})}", format=True)
        
        assert response.status_code == status.HTTP_401_UNAUTHORIZED

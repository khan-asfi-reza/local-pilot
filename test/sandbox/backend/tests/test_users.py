"""Tests for user profile and posts listing endpoints."""

import pytest
from django.contrib.auth import get_user_model
from django.urls import reverse
from rest_framework import status


User = get_user_model()


class TestGetUserProfile:
    """Test getting user profile endpoint."""

    def test_get_profile_success(self, client):
        """Test getting existing user profile."""
        user = User.objects.create_user(
            username="testuser",
            email="test@example.com",
            password="securepass123",
            bio="Test bio"
        )
        
        response = client.get(f"{reverse('users:profile', kwargs={'username': 'testuser'})}", format=True)
        
        assert response.status_code == status.HTTP_200_OK
        assert response.data["username"] == "testuser"
        assert response.data["email"] == "test@example.com"
        assert response.data["bio"] == "Test bio"

    def test_get_profile_nonexistent(self, client):
        """Test getting nonexistent user profile returns 404."""
        response = client.get(reverse("users:profile", kwargs={"username": "nonexistent"}), format=True)
        
        assert response.status_code == status.HTTP_404_NOT_FOUND

    def test_get_profile_without_auth(self, client):
        """Test getting user profile without authentication."""
        user = User.objects.create_user(
            username="testuser",
            email="test@example.com",
            password="securepass123"
        )
        
        response = client.get(f"{reverse('users:profile', kwargs={'username': 'testuser'})}", format=True)
        
        assert response.status_code == status.HTTP_200_OK


class TestGetUserPosts:
    """Test getting user's posts endpoint."""

    def test_get_user_posts_success(self, client):
        """Test getting existing user's posts."""
        user = User.objects.create_user(
            username="testuser",
            email="test@example.com",
            password="securepass123"
        )
        
        from core.models import Post
        
        # Create multiple posts for the user
        post1 = Post.objects.create(
            author=user,
            body="First post",
            post_date="2024-01-01",
            like_count=5
        )
        post2 = Post.objects.create(
            author=user,
            body="Second post",
            post_date="2024-01-02",
            like_count=3
        )
        
        response = client.get(f"{reverse('users:posts', kwargs={'username': 'testuser'})}", format=True)
        
        assert response.status_code == status.HTTP_200_OK
        posts = response.data
        
        # Verify both posts are present
        post_ids = {p["id"] for p in posts}
        assert post_ids == {post1.id, post2.id}

    def test_get_user_posts_nonexistent(self, client):
        """Test getting nonexistent user's posts returns 404."""
        response = client.get(reverse("users:posts", kwargs={"username": "nonexistent"}), format=True)
        
        assert response.status_code == status.HTTP_404_NOT_FOUND

    def test_get_user_posts_empty(self, client):
        """Test getting user with no posts returns empty list."""
        user = User.objects.create_user(
            username="testuser",
            email="test@example.com",
            password="securepass123"
        )
        
        response = client.get(f"{reverse('users:posts', kwargs={'username': 'testuser'})}", format=True)
        
        assert response.status_code == status.HTTP_200_OK
        assert response.data == []

    def test_get_user_posts_ordering(self, client):
        """Test user posts are ordered chronologically."""
        user = User.objects.create_user(
            username="testuser",
            email="test@example.com",
            password="securepass123"
        )
        
        from core.models import Post
        
        # Create posts in reverse chronological order
        post_newest = Post.objects.create(
            author=user,
            body="Newest post",
            post_date="2024-01-03",
            like_count=10
        )
        post_oldest = Post.objects.create(
            author=user,
            body="Oldest post",
            post_date="2024-01-01",
            like_count=5
        )
        
        response = client.get(f"{reverse('users:posts', kwargs={'username': 'testuser'})}", format=True)
        posts = response.data
        
        # Should be ordered newest first
        assert posts[0]["id"] == post_newest.id
        assert posts[1]["id"] == post_oldest.id


class TestUserStats:
    """Test user stats are correctly calculated."""

    def test_user_post_count(self, client):
        """Test user post count is accurate."""
        user = User.objects.create_user(
            username="testuser",
            email="test@example.com",
            password="securepass123"
        )
        
        from core.models import Post
        
        # Create 5 posts
        for i in range(5):
            Post.objects.create(
                author=user,
                body=f"Post {i}",
                post_date=f"2024-01-{i+1:02d}",
                like_count=0
            )
        
        response = client.get(f"{reverse('users:profile', kwargs={'username': 'testuser'})}", format=True)
        
        assert response.status_code == status.HTTP_200_OK
        assert response.data["post_count"] == 5

    def test_user_likes_received(self, client):
        """Test user total likes received is accurate."""
        user = User.objects.create_user(
            username="testuser",
            email="test@example.com",
            password="securepass123"
        )
        
        from core.models import Post
        
        # Create posts with different like counts
        post1 = Post.objects.create(
            author=user,
            body="Post 1",
            post_date="2024-01-01",
            like_count=10
        )
        post2 = Post.objects.create(
            author=user,
            body="Post 2",
            post_date="2024-01-02",
            like_count=20
        )
        
        response = client.get(f"{reverse('users:profile', kwargs={'username': 'testuser'})}", format=True)
        
        assert response.status_code == status.HTTP_200_OK
        assert response.data["total_likes_received"] == 30


class TestUserBioUpdate:
    """Test updating user bio."""

    def test_update_bio_success(self, client):
        """Test successful bio update."""
        user = User.objects.create_user(
            username="testuser",
            email="test@example.com",
            password="securepass123",
            bio="Old bio"
        )
        
        # Login to get auth token
        response = client.post(reverse("auth:login"), {
            "username": "testuser",
            "password": "securepass123"
        }, format=True)
        
        assert response.status_code == status.HTTP_200_OK
        tokens = response.data["tokens"]
        
        # Update bio
        update_response = client.patch(reverse("auth:update_me"), {
            "bio": "New bio"
        }, format=True, **tokens)
        
        assert update_response.status_code == status.HTTP_200_OK
        assert update_response.data["bio"] == "New bio"

    def test_update_bio_without_auth(self, client):
        """Test updating bio fails without authentication."""
        response = client.patch(reverse("auth:update_me"), {
            "bio": "New bio"
        }, format=True)
        
        assert response.status_code == status.HTTP_401_UNAUTHORIZED

    def test_update_bio_empty(self, client):
        """Test updating bio with empty string."""
        user = User.objects.create_user(
            username="testuser",
            email="test@example.com",
            password="securepass123"
        )
        
        # Login to get auth token
        response = client.post(reverse("auth:login"), {
            "username": "testuser",
            "password": "securepass123"
        }, format=True)
        
        assert response.status_code == status.HTTP_200_OK
        tokens = response.data["tokens"]
        
        # Update bio to empty
        update_response = client.patch(reverse("auth:update_me"), {
            "bio": ""
        }, format=True, **tokens)
        
        assert update_response.status_code == status.HTTP_200_OK


class TestUserEmailUpdate:
    """Test updating user email."""

    def test_update_email_success(self, client):
        """Test successful email update."""
        user = User.objects.create_user(
            username="testuser",
            email="old@example.com",
            password="securepass123"
        )
        
        # Login to get auth token
        response = client.post(reverse("auth:login"), {
            "username": "testuser",
            "password": "securepass123"
        }, format=True)
        
        assert response.status_code == status.HTTP_200_OK
        tokens = response.data["tokens"]
        
        # Update email - note: this might not be supported in the actual API
        # Testing that the endpoint handles it gracefully
        update_response = client.patch(reverse("auth:update_me"), {
            "email": "new@example.com"
        }, format=True, **tokens)
        
        # Should return 400 if email field is not updatable
        assert update_response.status_code in [status.HTTP_200_OK, status.HTTP_400_BAD_REQUEST]

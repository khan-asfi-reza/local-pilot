"""Tests for authentication endpoints."""

import pytest
from django.contrib.auth import get_user_model
from django.urls import reverse
from rest_framework import status


User = get_user_model()


class TestAuthSignup:
    """Test user signup endpoint."""

    def test_signup_success(self, client):
        """Test successful user registration."""
        data = {
            "username": "testuser",
            "email": "test@example.com",
            "password": "securepass123"
        }
        response = client.post(reverse("auth:signup"), data, format=True)
        
        assert response.status_code == status.HTTP_201_CREATED
        assert response.data["username"] == "testuser"
        assert response.data["email"] == "test@example.com"
        assert "tokens" in response.data

    def test_signup_duplicate_username(self, client):
        """Test signup fails with duplicate username."""
        # Create first user
        User.objects.create_user(
            username="existing",
            email="existing@example.com",
            password="pass123"
        )
        
        data = {
            "username": "existing",
            "email": "new@example.com",
            "password": "pass456"
        }
        response = client.post(reverse("auth:signup"), data, format=True)
        
        assert response.status_code == status.HTTP_400_BAD_REQUEST
        assert "username" in response.data

    def test_signup_duplicate_email(self, client):
        """Test signup fails with duplicate email."""
        # Create first user
        User.objects.create_user(
            username="user1",
            email="taken@example.com",
            password="pass123"
        )
        
        data = {
            "username": "user2",
            "email": "taken@example.com",
            "password": "pass456"
        }
        response = client.post(reverse("auth:signup"), data, format=True)
        
        assert response.status_code == status.HTTP_400_BAD_REQUEST
        assert "email" in response.data

    def test_signup_missing_fields(self, client):
        """Test signup fails with missing required fields."""
        data = {
            "username": "testuser",
            # Missing email and password
        }
        response = client.post(reverse("auth:signup"), data, format=True)
        
        assert response.status_code == status.HTTP_400_BAD_REQUEST

    def test_signup_weak_password(self, client):
        """Test signup fails with weak password."""
        data = {
            "username": "testuser",
            "email": "test@example.com",
            "password": "123"
        }
        response = client.post(reverse("auth:signup"), data, format=True)
        
        assert response.status_code == status.HTTP_400_BAD_REQUEST


class TestAuthLogin:
    """Test user login endpoint."""

    def test_login_success(self, client):
        """Test successful user login."""
        User.objects.create_user(
            username="testuser",
            email="test@example.com",
            password="securepass123"
        )
        
        data = {
            "username": "testuser",
            "password": "securepass123"
        }
        response = client.post(reverse("auth:login"), data, format=True)
        
        assert response.status_code == status.HTTP_200_OK
        assert response.data["username"] == "testuser"
        assert "tokens" in response.data

    def test_login_wrong_password(self, client):
        """Test login fails with wrong password."""
        User.objects.create_user(
            username="testuser",
            email="test@example.com",
            password="securepass123"
        )
        
        data = {
            "username": "testuser",
            "password": "wrongpassword"
        }
        response = client.post(reverse("auth:login"), data, format=True)
        
        assert response.status_code == status.HTTP_401_UNAUTHORIZED

    def test_login_nonexistent_user(self, client):
        """Test login fails with nonexistent user."""
        data = {
            "username": "nonexistent",
            "password": "pass123"
        }
        response = client.post(reverse("auth:login"), data, format=True)
        
        assert response.status_code == status.HTTP_401_UNAUTHORIZED

    def test_login_missing_credentials(self, client):
        """Test login fails with missing credentials."""
        data = {
            "username": "testuser",
            # Missing password
        }
        response = client.post(reverse("auth:login"), data, format=True)
        
        assert response.status_code == status.HTTP_400_BAD_REQUEST


class TestAuthRefresh:
    """Test token refresh endpoint."""

    def test_refresh_success(self, client):
        """Test successful token refresh."""
        # Create user and login to get initial tokens
        user = User.objects.create_user(
            username="testuser",
            email="test@example.com",
            password="securepass123"
        )
        
        response = client.post(reverse("auth:login"), {
            "username": "testuser",
            "password": "securepass123"
        }, format=True)
        
        assert response.status_code == status.HTTP_200_OK
        tokens = response.data["tokens"]
        
        # Refresh token
        refresh_response = client.post(reverse("auth:refresh"), format=True, **tokens)
        
        assert refresh_response.status_code == status.HTTP_200_OK
        assert "tokens" in refresh_response.data

    def test_refresh_without_auth(self, client):
        """Test refresh fails without authentication."""
        response = client.post(reverse("auth:refresh"), format=True)
        
        assert response.status_code == status.HTTP_401_UNAUTHORIZED


class TestAuthLogout:
    """Test logout endpoint."""

    def test_logout_success(self, client):
        """Test successful logout."""
        # Create user and login
        user = User.objects.create_user(
            username="testuser",
            email="test@example.com",
            password="securepass123"
        )
        
        response = client.post(reverse("auth:login"), {
            "username": "testuser",
            "password": "securepass123"
        }, format=True)
        
        assert response.status_code == status.HTTP_200_OK
        tokens = response.data["tokens"]
        
        # Logout
        logout_response = client.post(reverse("auth:logout"), format=True, **tokens)
        
        assert logout_response.status_code == status.HTTP_200_OK

    def test_logout_without_auth(self, client):
        """Test logout fails without authentication."""
        response = client.post(reverse("auth:logout"), format=True)
        
        assert response.status_code == status.HTTP_401_UNAUTHORIZED


class TestAuthMe:
    """Test get current user endpoint."""

    def test_me_success(self, client):
        """Test getting current user profile."""
        # Create user and login
        user = User.objects.create_user(
            username="testuser",
            email="test@example.com",
            password="securepass123"
        )
        
        response = client.post(reverse("auth:login"), {
            "username": "testuser",
            "password": "securepass123"
        }, format=True)
        
        assert response.status_code == status.HTTP_200_OK
        tokens = response.data["tokens"]
        
        # Get current user
        me_response = client.get(reverse("auth:me"), format=True, **tokens)
        
        assert me_response.status_code == status.HTTP_200_OK
        assert me_response.data["username"] == "testuser"
        assert me_response.data["email"] == "test@example.com"

    def test_me_without_auth(self, client):
        """Test get current user fails without authentication."""
        response = client.get(reverse("auth:me"), format=True)
        
        assert response.status_code == status.HTTP_401_UNAUTHORIZED

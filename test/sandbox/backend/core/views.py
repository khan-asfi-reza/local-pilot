from django.shortcuts import render
from .views.auth import (
    signup,
    login,
    refresh,
    logout,
    me,
    update_me
)
from .views.users import get_user_profile, get_user_posts

# Re-export auth views for convenience
__all__ = [
    'signup',
    'login', 
    'refresh',
    'logout',
    'me',
    'update_me',
    'get_user_profile',
    'get_user_posts'
]

from django.contrib import admin
from django.urls import path, include
from core.views.auth import signup, login, refresh, logout, me, update_me
from core.views.posts import feed, create_post, get_post, delete_post, like_post, unlike_post
from core.views.users import get_user_profile, get_user_posts

urlpatterns = [
    path('admin/', admin.site.urls),
    path('api/auth/signup/', signup),
    path('api/auth/login/', login),
    path('api/auth/refresh/', refresh),
    path('api/auth/logout/', logout),
    path('api/auth/me/', me),
    path('api/auth/update-me/', update_me),
    # User profile and posts
    path('api/v1/users/<str:username>/', get_user_profile),
    path('api/v1/users/<str:username>/posts/', get_user_posts),
    # Feed and posts
    path('api/v1/feed/', feed),
    path('api/v1/posts/', create_post),
    path('api/v1/posts/<int:pk>/', get_post),
    path('api/v1/posts/<int:pk>/delete/', delete_post),
    path('api/v1/posts/<int:pk>/like/', like_post),
    path('api/v1/posts/<int:pk>/unlike/', unlike_post),
]

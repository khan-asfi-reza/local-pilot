from django.http import JsonResponse
from django.contrib.auth.models import User
from rest_framework.decorators import api_view, permission_classes
from rest_framework.permissions import IsAuthenticated
from ..models import Post


@api_view(['GET'])
def get_user_profile(request, username):
    """Get user profile and stats"""
    try:
        user = User.objects.get(username=username)
    except User.DoesNotExist:
        return JsonResponse({'error': 'User not found'}, status=404)

    # Calculate post count and total likes received
    posts = Post.objects.filter(author=user).exclude(is_deleted=True)
    post_count = posts.count()
    
    # Sum up all likes from the Like model
    total_likes_received = 0
    for post in posts:
        total_likes_received += post.like_count

    return JsonResponse({
        'id': user.id,
        'username': user.username,
        'email': user.email,
        'bio': user.bio,
        'created_at': user.date_joined.isoformat(),
        'post_count': post_count,
        'total_likes_received': total_likes_received
    })


@api_view(['GET'])
def get_user_posts(request, username):
    """Get user's chronological posts"""
    try:
        user = User.objects.get(username=username)
    except User.DoesNotExist:
        return JsonResponse({'error': 'User not found'}, status=404)

    # Get all non-deleted posts for this user, ordered by created_at descending (newest first)
    posts = Post.objects.filter(author=user).exclude(is_deleted=True).order_by('-created_at')

    post_list = []
    for post in posts:
        post_data = {
            'id': post.id,
            'author_id': post.author.id,
            'body': post.body,
            'created_at': post.created_at.isoformat(),
            'post_date': post.post_date.strftime('%Y-%m-%d'),
            'like_count': post.like_count,
            'is_deleted': post.is_deleted,
            'author_username': post.author.username
        }
        post_list.append(post_data)

    return JsonResponse({'posts': post_list}, safe=False)

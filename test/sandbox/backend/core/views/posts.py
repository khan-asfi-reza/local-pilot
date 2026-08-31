"""Feed and post views for CRUD operations and likes."""
from django.db.models import Count
from rest_framework.decorators import api_view, permission_classes
from rest_framework.permissions import IsAuthenticated
from rest_framework.response import Response
from ..models import Post, Like
from ..serializers.posts import PostCreateSerializer


@api_view(['GET'])
def feed(request):
    """Get feed posts ordered by day/likes."""
    # Query posts that are not deleted, ordered by post_date desc then like_count desc
    posts = (Post.objects.filter(is_deleted=False)
              .annotate(like_count=Count('likes'))
              .order_by('-post_date', '-like_count'))
    
    results = []
    for post in posts:
        try:
            author_username = post.author.username
        except (AttributeError, Exception):
            author_username = 'unknown'
        
        results.append({
            'id': post.id,
            'author_id': post.author_id,
            'body': post.body,
            'created_at': post.created_at,
            'post_date': post.post_date,
            'like_count': post.like_count,
            'is_deleted': False,
            'author_username': author_username
        })
    
    return Response(results)


@api_view(['POST'])
@permission_classes([IsAuthenticated])
def create_post(request):
    """Create a new post."""
    serializer = PostCreateSerializer(data=request.data)
    if serializer.is_valid():
        # Create post with current user as author
        post = Post.objects.create(
            author=request.user,
            body=serializer.validated_data['body']
        )
        
        # Get the serialized result
        try:
            author_username = post.author.username
        except (AttributeError, Exception):
            author_username = 'unknown'
        
        return Response({
            'id': post.id,
            'author_id': post.author_id,
            'body': post.body,
            'created_at': post.created_at,
            'post_date': post.post_date,
            'like_count': 0,
            'is_deleted': False,
            'author_username': author_username
        }, status=201)
    
    return Response(serializer.errors, status=400)


@api_view(['GET'])
def get_post(request, pk):
    """Get a single post by ID."""
    try:
        post = Post.objects.get(id=pk, is_deleted=False)
    except Post.DoesNotExist:
        return Response({'error': 'Post not found'}, status=404)
    
    try:
        author_username = post.author.username
    except (AttributeError, Exception):
        author_username = 'unknown'
    
    return Response({
        'id': post.id,
        'author_id': post.author_id,
        'body': post.body,
        'created_at': post.created_at,
        'post_date': post.post_date,
        'like_count': post.like_count,
        'is_deleted': False,
        'author_username': author_username
    })


@api_view(['DELETE'])
@permission_classes([IsAuthenticated])
def delete_post(request, pk):
    """Soft delete a post (author only)."""
    try:
        post = Post.objects.get(id=pk)
    except Post.DoesNotExist:
        return Response({'error': 'Post not found'}, status=404)
    
    # Only allow author to delete their own post
    if post.author != request.user:
        return Response({'error': 'Not authorized'}, status=403)
    
    post.is_deleted = True
    post.save()
    
    return Response({
        'id': post.id,
        'body': post.body,
        'created_at': post.created_at,
        'is_deleted': True
    })


@api_view(['POST'])
@permission_classes([IsAuthenticated])
def like_post(request, pk):
    """Like a post (idempotent)."""
    try:
        post = Post.objects.get(id=pk)
    except Post.DoesNotExist:
        return Response({'error': 'Post not found'}, status=404)
    
    # Check if user already liked the post
    try:
        existing_like = Like.objects.get(post=post, user=request.user)
        # User already liked, return current like count
        return Response({
            'post_id': post.id,
            'like_count': post.like_count
        })
    except Like.DoesNotExist:
        pass
    
    # Create new like
    Like.objects.create(post=post, user=request.user)
    
    # Refresh post to get updated like count
    post.refresh_from_db()
    
    return Response({
        'post_id': post.id,
        'like_count': post.like_count
    })


@api_view(['DELETE'])
@permission_classes([IsAuthenticated])
def unlike_post(request, pk):
    """Unlike a post (idempotent)."""
    try:
        post = Post.objects.get(id=pk)
    except Post.DoesNotExist:
        return Response({'error': 'Post not found'}, status=404)
    
    # Check if user liked the post
    try:
        existing_like = Like.objects.get(post=post, user=request.user)
    except Like.DoesNotExist:
        return Response({
            'post_id': post.id,
            'like_count': post.like_count - 1
        })
    
    # Delete the like
    existing_like.delete()
    
    # Refresh post to get updated like count
    post.refresh_from_db()
    
    return Response({
        'post_id': post.id,
        'like_count': post.like_count
    })

"""Authentication views for JWT-based auth."""
import jwt
from django.conf import settings
from django.contrib.auth import authenticate, login as django_login
from django.contrib.auth.hashers import check_password
from django.http import JsonResponse
from rest_framework.decorators import api_view, permission_classes
from rest_framework.permissions import IsAuthenticated
from rest_framework.response import Response
from ..models import User
from ..serializers.auth import AuthSerializer


@api_view(['POST'])
def signup(request):
    """Register a new user account."""
    serializer = AuthSerializer(data=request.data)
    if serializer.is_valid():
        username = serializer.validated_data['username']
        email = serializer.validated_data['email']
        password = serializer.validated_data['password']
        
        # Check if user exists
        if User.objects.filter(username=username).exists() or User.objects.filter(email=email).exists():
            return Response(
                {'error': 'Username or email already exists'},
                status=400
            )
        
        # Create user
        user = User.objects.create_user(
            username=username,
            email=email,
            password=password,
            bio=''
        )
        
        # Generate tokens
        access_token = jwt.encode({
            'user_id': user.id,
            'type': 'access',
            'exp': None  # No expiration for access token
        }, settings.SECRET_KEY, algorithm='HS256')
        
        refresh_token = jwt.encode({
            'user_id': user.id,
            'type': 'refresh',
            'exp': None  # No expiration for refresh token
        }, settings.SECRET_KEY, algorithm='HS256')
        
        return Response({
            'id': user.id,
            'username': user.username,
            'email': user.email,
            'bio': user.bio,
            'created_at': user.created_at,
            'tokens': {
                'access': access_token,
                'refresh': refresh_token
            }
        }, status=201)
    
    return Response(serializer.errors, status=400)


@api_view(['POST'])
def login(request):
    """Authenticate user and receive tokens."""
    username = request.data.get('username')
    password = request.data.get('password')
    
    if not username or not password:
        return Response({'error': 'Username and password required'}, status=400)
    
    # Authenticate user
    user = authenticate(username=username, password=password)
    if not user:
        return Response({'error': 'Invalid credentials'}, status=401)
    
    # Generate tokens
    access_token = jwt.encode({
        'user_id': user.id,
        'type': 'access',
        'exp': None
    }, settings.SECRET_KEY, algorithm='HS256')
    
    refresh_token = jwt.encode({
        'user_id': user.id,
        'type': 'refresh',
        'exp': None
    }, settings.SECRET_KEY, algorithm='HS256')
    
    return Response({
        'id': user.id,
        'username': user.username,
        'email': user.email,
        'bio': user.bio,
        'created_at': user.created_at,
        'tokens': {
            'access': access_token,
            'refresh': refresh_token
        }
    })


@api_view(['POST'])
@permission_classes([IsAuthenticated])
def refresh(request):
    """Rotate refresh token."""
    # Decode the current refresh token from request
    try:
        current_refresh = jwt.decode(
            request.data.get('refresh', ''),
            settings.SECRET_KEY,
            algorithms=['HS256']
        )
    except jwt.ExpiredSignatureError:
        return Response({'error': 'Refresh token expired'}, status=401)
    except jwt.InvalidTokenError:
        return Response({'error': 'Invalid refresh token'}, status=401)
    
    # Get user from token
    user_id = current_refresh.get('user_id')
    if not user_id:
        return Response({'error': 'Invalid token'}, status=401)
    
    try:
        user = User.objects.get(id=user_id)
    except User.DoesNotExist:
        return Response({'error': 'User not found'}, status=401)
    
    # Generate new tokens
    access_token = jwt.encode({
        'user_id': user.id,
        'type': 'access',
        'exp': None
    }, settings.SECRET_KEY, algorithm='HS256')
    
    refresh_token = jwt.encode({
        'user_id': user.id,
        'type': 'refresh',
        'exp': None
    }, settings.SECRET_KEY, algorithm='HS256')
    
    return Response({'tokens': {
        'access': access_token,
        'refresh': refresh_token
    }})


@api_view(['POST'])
@permission_classes([IsAuthenticated])
def logout(request):
    """Logout user and blacklist refresh token."""
    # Decode the current refresh token from request
    try:
        current_refresh = jwt.decode(
            request.data.get('refresh', ''),
            settings.SECRET_KEY,
            algorithms=['HS256']
        )
    except (jwt.ExpiredSignatureError, jwt.InvalidTokenError):
        # If no refresh token or expired, still allow logout
        pass
    
    # Get user from token if provided
    user_id = current_refresh.get('user_id') if 'current_refresh' in locals() else request.user.id
    
    try:
        user = User.objects.get(id=user_id)
    except User.DoesNotExist:
        return Response({'error': 'User not found'}, status=401)
    
    # Blacklist the refresh token by deleting it from database
    # Store blacklisted tokens in a separate table or cache
    # For simplicity, we'll use Django's session storage
    from django.core.cache import cache
    
    # Create a blacklist entry with TTL equal to refresh token lifetime
    if user_id:
        cache.set(f'blacklist:{user_id}', '1', 86400)  # 24 hours
    
    return Response({})


@api_view(['GET'])
@permission_classes([IsAuthenticated])
def me(request):
    """Get current authenticated user profile."""
    user = request.user
    
    # Count posts and likes for this user
    post_count = Post.objects.filter(author=user).count()
    total_likes_received = Like.objects.filter(post__author=user).count()
    
    return Response({
        'id': user.id,
        'username': user.username,
        'email': user.email,
        'bio': user.bio,
        'created_at': user.created_at,
        'post_count': post_count,
        'total_likes_received': total_likes_received
    })


@api_view(['PATCH'])
@permission_classes([IsAuthenticated])
def update_me(request):
    """Update current user's bio."""
    serializer = AuthSerializer(
        instance=request.user,
        data={'bio': request.data.get('bio', '')}
    )
    if serializer.is_valid():
        serializer.save()
        
        return Response({
            'id': serializer.instance.id,
            'username': serializer.instance.username,
            'email': serializer.instance.email,
            'bio': serializer.instance.bio,
            'created_at': serializer.instance.created_at,
            'post_count': Post.objects.filter(author=serializer.instance).count(),
            'total_likes_received': Like.objects.filter(post__author=serializer.instance).count()
        })
    
    return Response(serializer.errors, status=400)



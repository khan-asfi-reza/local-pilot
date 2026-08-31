"""Middleware for JWT token validation."""
import jwt
from django.conf import settings
from django.http import JsonResponse
from rest_framework.exceptions import AuthenticationFailed


def get_token_from_request(request):
    """Extract JWT token from request headers or cookies."""
    # Check Authorization header first
    auth_header = request.META.get('HTTP_AUTHORIZATION', '')
    if auth_header.startswith('Bearer '):
        return auth_header[7:]
    
    # Fall back to cookie
    token = request.COOKIES.get('access_token')
    if token:
        return token
    
    return None


class JWTAuthentication:
    """Custom authentication class for JWT tokens."""
    
    def authenticate(self, request):
        """Authenticate user using JWT token."""
        token = get_token_from_request(request)
        
        if not token:
            return None
        
        try:
            # Decode and validate token
            payload = jwt.decode(
                token,
                settings.SECRET_KEY,
                algorithms=['HS256']
            )
            
            user_id = payload.get('user_id')
            if not user_id:
                return None
            
            from ..models import User
            try:
                user = User.objects.get(id=user_id)
                # Check if token is blacklisted
                from django.core.cache import cache
                if cache.get(f'blacklist:{user_id}'):
                    return None
                
                return {
                    'user': user,
                    'token': token
                }
            except User.DoesNotExist:
                return None
                
        except jwt.ExpiredSignatureError:
            # Token expired, check for refresh token in request
            refresh_token = get_token_from_request(request)
            if refresh_token:
                try:
                    payload = jwt.decode(
                        refresh_token,
                        settings.SECRET_KEY,
                        algorithms=['HS256']
                    )
                    user_id = payload.get('user_id')
                    if not user_id:
                        return None
                    
                    from ..models import User
                    try:
                        user = User.objects.get(id=user_id)
                        # Check blacklist
                        from django.core.cache import cache
                        if cache.get(f'blacklist:{user_id}'):
                            return None
                        
                        return {
                            'user': user,
                            'token': refresh_token
                        }
                    except User.DoesNotExist:
                        return None
                except (jwt.ExpiredSignatureError, jwt.InvalidTokenError):
                    pass
            return None
            
        except jwt.InvalidTokenError:
            return None
    
    def authenticate_header(self, request):
        """Return the WWW-Authenticate header for 401 responses."""
        return 'Bearer realm="API"'


def authentication_middleware(get_response):
    """Middleware to handle JWT authentication for all requests."""
    
    def middleware(request):
        # Skip authentication for auth endpoints
        auth_paths = [
            '/api/auth/signup',
            '/api/auth/login',
            '/api/auth/refresh',
            '/api/auth/logout',
            '/api/auth/me',
            '/api/auth/update-me'
        ]
        
        path = request.path
        
        # Check if this is an auth endpoint
        for auth_path in auth_paths:
            if path.startswith(auth_path):
                return get_response(request)
        
        # For other endpoints, check authentication
        token = get_token_from_request(request)
        
        if not token:
            response = JsonResponse({
                'error': 'Authentication required',
                'message': 'No valid token found'
            }, status=401)
            return response
        
        try:
            payload = jwt.decode(
                token,
                settings.SECRET_KEY,
                algorithms=['HS256']
            )
            
            user_id = payload.get('user_id')
            if not user_id:
                response = JsonResponse({
                    'error': 'Invalid token',
                    'message': 'Token does not contain valid user ID'
                }, status=401)
                return response
            
            from ..models import User
            try:
                user = User.objects.get(id=user_id)
                
                # Check blacklist
                from django.core.cache import cache
                if cache.get(f'blacklist:{user_id}'):
                    response = JsonResponse({
                        'error': 'Token revoked',
                        'message': 'Your session has been terminated'
                    }, status=401)
                    return response
                
            except User.DoesNotExist:
                response = JsonResponse({
                    'error': 'Invalid token',
                    'message': 'User not found for this token'
                }, status=401)
                return response
            
        except jwt.ExpiredSignatureError:
            # Token expired, check for refresh token
            from django.core.cache import cache
            user_id = payload.get('user_id') if 'payload' in locals() else None
            
            # Try to get refresh token and rotate it
            refresh_token = request.COOKIES.get('refresh_token')
            if refresh_token:
                try:
                    new_payload = jwt.decode(
                        refresh_token,
                        settings.SECRET_KEY,
                        algorithms=['HS256']
                    )
                    
                    from ..models import User
                    user = User.objects.get(id=new_payload.get('user_id'))
                    
                    # Check blacklist
                    if cache.get(f'blacklist:{new_payload.get("user_id")}'):
                        response = JsonResponse({
                            'error': 'Token revoked',
                            'message': 'Your session has been terminated'
                        }, status=401)
                        return response
                    
                    # Generate new tokens
                    import jwt as jwt_lib
                    from django.conf import settings
                    
                    access_token = jwt_lib.encode({
                        'user_id': user.id,
                        'type': 'access',
                        'exp': None
                    }, settings.SECRET_KEY, algorithm='HS256')
                    
                    refresh_token = jwt_lib.encode({
                        'user_id': user.id,
                        'type': 'refresh',
                        'exp': None
                    }, settings.SECRET_KEY, algorithm='HS256')
                    
                    response = JsonResponse({
                        'access_token': access_token,
                        'refresh_token': refresh_token
                    })
                    return response
                    
                except (jwt.ExpiredSignatureError, jwt.InvalidTokenError):
                    pass
            
            response = JsonResponse({
                'error': 'Token expired',
                'message': 'Please login again'
            }, status=401)
            return response
            
        except jwt.InvalidTokenError:
            response = JsonResponse({
                'error': 'Invalid token',
                'message': 'The provided token is invalid'
            }, status=401)
            return response
        
        # If we get here, user is authenticated
        request.user = user
        return get_response(request)
    
    return middleware


def add_auth_headers(response):
    """Add authentication headers to response."""
    from django.conf import settings
    
    if hasattr(response, 'headers'):
        response['X-Request-ID'] = str(id(response))
    
    return response

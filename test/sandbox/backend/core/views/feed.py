"""Feed view - consolidated into views/posts.py."""
from rest_framework.decorators import api_view, permission_classes
from rest_framework.permissions import IsAuthenticated
from rest_framework.response import Response
from ..models import Post


@api_view(['GET'])
def get_feed(request):
    """Get feed posts ordered by day/likes.

    This view is now consolidated into views/posts.py as the `feed` function.
    Kept here for backward compatibility if needed.
    """
    return Response([])  # Use views/posts.py/feed instead

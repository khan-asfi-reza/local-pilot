"""Post serializers for API responses."""
from rest_framework import serializers
from django.contrib.auth.models import User
from ..models import Post


class PostSerializer(serializers.ModelSerializer):
    """Serialize post data with author info and like count."""
    
    class Meta:
        model = Post
        fields = ['id', 'author_id', 'body', 'created_at', 'post_date', 
                  'like_count', 'is_deleted', 'author_username']
        read_only_fields = ['author_id', 'created_at', 'post_date', 'like_count', 'is_deleted']

    def to_representation(self, instance):
        """Add author username and filter deleted posts."""
        representation = super().to_representation(instance)
        
        # Get author username if not deleted
        if not instance.is_deleted:
            try:
                representation['author_username'] = instance.author.username
            except (AttributeError, User.DoesNotExist):
                representation['author_username'] = 'unknown'
        
        return representation


class PostCreateSerializer(serializers.ModelSerializer):
    """Serializer for creating posts."""
    
    class Meta:
        model = Post
        fields = ['body']

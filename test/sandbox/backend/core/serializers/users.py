from rest_framework import serializers
from django.contrib.auth.models import User
from ..models import Post


class UserProfileSerializer(serializers.ModelSerializer):
    """Serializer for user profile with stats"""
    post_count = serializers.IntegerField(read_only=True)
    total_likes_received = serializers.IntegerField(read_only=True)

    class Meta:
        model = User
        fields = ['id', 'username', 'email', 'bio', 'created_at', 'post_count', 'total_likes_received']
        read_only_fields = ['id', 'username', 'email', 'created_at', 'post_count', 'total_likes_received']

    def to_representation(self, instance):
        # Calculate post count and total likes received
        posts = Post.objects.filter(author=instance).exclude(is_deleted=True)
        post_count = posts.count()
        
        # Sum up all likes from the Like model
        total_likes_received = 0
        for post in posts:
            total_likes_received += post.like_count

        data = super().to_representation(instance)
        data['post_count'] = post_count
        data['total_likes_received'] = total_likes_received
        return data


class UserPostsListSerializer(serializers.Serializer):
    """Serializer for user posts list"""
    posts = serializers.ListField(child=serializers.DictField())

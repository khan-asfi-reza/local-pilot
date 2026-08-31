"""Serializers for authentication operations."""
from rest_framework import serializers
from ..models import User


class AuthSerializer(serializers.ModelSerializer):
    """Serializer for user authentication operations."""
    
    class Meta:
        model = User
        fields = ['id', 'username', 'email', 'bio', 'created_at']
        read_only_fields = ['id', 'email', 'created_at']
        
    def create(self, validated_data):
        """Create a new user."""
        validated_data['password'] = self.context.get('password')
        return User.objects.create_user(**validated_data)
    
    def update(self, instance, validated_data):
        """Update user bio."""
        if 'bio' in validated_data:
            instance.bio = validated_data['bio']
            instance.save()
        return instance


class TokenSerializer(serializers.Serializer):
    """Serializer for token operations."""
    
    access_token = serializers.CharField(read_only=True)
    refresh_token = serializers.CharField(read_only=True)
    
    def validate(self, data):
        """Validate tokens are present."""
        if not data.get('access_token') or not data.get('refresh_token'):
            raise serializers.ValidationError({
                'non_field_errors': 'Both access and refresh tokens are required'
            })
        return data


class RefreshTokenSerializer(serializers.Serializer):
    """Serializer for refresh token rotation."""
    
    refresh = serializers.CharField(required=True)
    
    def validate(self, data):
        """Validate refresh token."""
        try:
            import jwt
            from django.conf import settings
            
            jwt.decode(data['refresh'], settings.SECRET_KEY, algorithms=['HS256'])
        except (jwt.ExpiredSignatureError, jwt.InvalidTokenError):
            raise serializers.ValidationError({
                'refresh': 'Invalid or expired refresh token'
            })
        return data

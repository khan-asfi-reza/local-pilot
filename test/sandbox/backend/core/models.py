from django.db import models
from django.contrib.auth.models import AbstractUser, Group, Permission


class User(AbstractUser):
    """User model extending AbstractUser with additional fields."""
    email = models.EmailField(unique=True)
    bio = models.TextField(blank=True, null=True, default="")

    groups = models.ManyToManyField(
        Group,
        related_name='core_user_groups',
        blank=True,
        help_text='The groups this user belongs to.',
    )
    user_permissions = models.ManyToManyField(
        Permission,
        related_name='core_user_permissions',
        blank=True,
        help_text='All permissions for this user.',
    )

    def __str__(self):
        return self.username


class Post(models.Model):
    """Post model for social media content."""
    author = models.ForeignKey(User, on_delete=models.CASCADE, related_name='posts')
    body = models.TextField()
    post_date = models.DateTimeField(auto_now_add=True)
    like_count = models.PositiveIntegerField(default=0)
    is_deleted = models.BooleanField(default=False)

    class Meta:
        ordering = ['-post_date']

    def __str__(self):
        return f'Post by {self.author.username}'


class Like(models.Model):
    """Like model for tracking likes on posts."""
    post = models.ForeignKey(Post, on_delete=models.CASCADE, related_name='likes')
    user = models.ForeignKey(User, on_delete=models.CASCADE, related_name='likes_given')

    class Meta:
        unique_together = ['post', 'user']

    def __str__(self):
        return f'Like by {self.user.username} on {self.post}'

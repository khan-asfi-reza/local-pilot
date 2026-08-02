# Social app: Django + DRF backend, React frontend

Build a small social network: a Django REST Framework backend and a React
frontend.

## Backend (Django + DRF)
Create a Django project with an app that provides a token-authenticated REST API:
- User registration: `POST /api/register/` (username, password) creates a user.
- Login: `POST /api/login/` returns an auth token.
- Posts: `POST /api/posts/` (authenticated) creates a post; `GET /api/posts/`
  lists posts.
- Follow: `POST /api/follow/<user_id>/` (authenticated) follows another user.
- Feed: `GET /api/feed/` (authenticated) returns posts by users the current user
  follows.
- Include `requirements.txt` (pin `django`, `djangorestframework`) and generate
  the database migrations.
- Add a Django `TestCase`/`APITestCase` that exercises register → login → create
  post → follow → feed. It must pass with `python manage.py test`.

## Frontend (React)
A React frontend (a single-file React-via-CDN page is fine, or a Vite app) with:
register/login forms, a box to create a post, and a feed view. Wire it to the
API with the auth token.

## Verify
`python manage.py test` passes.

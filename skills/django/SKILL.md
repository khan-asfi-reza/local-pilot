---
name: django
description: Django project conventions.
internal: true
---
# Django
- Respect the layout: project package (settings/urls/wsgi) and apps (models/views/urls/migrations). Register apps in `INSTALLED_APPS`.
- After model changes: `python manage.py makemigrations` then `migrate`. Never hand-edit the DB.
- Wire views through `urls.py`. Keep `requirements.txt` current (Django + any packages).
- Verify: `python manage.py check`, run tests with `python manage.py test`. Don't leave `runserver` blocking — use the serve tool if a live check is needed.

## DRF: make every endpoint actually respond (not 500)
- `python manage.py check` passing does NOT mean the API works — a ViewSet can still 500 on the first request. Boot and GET each `/api/...` endpoint.
- **Every `ModelViewSet` MUST set `queryset = Model.objects.all()` AND `serializer_class = ...`.** Missing `queryset` raises `AssertionError: should either include a queryset attribute...` — a 500 on every list call. This is the most common break.
- Register ViewSets on a `DefaultRouter` and `include(router.urls)` under the `api/` prefix in `config/urls.py`. A registered-but-unincluded router 404s.
- Serializer `fields` must be real model fields; a typo'd field name 500s at request time, not at import.
- Put `rest_framework` (and `corsheaders` if used) in `INSTALLED_APPS`, and enable CORS so the Vite frontend can call `/api/...`.

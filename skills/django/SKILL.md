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

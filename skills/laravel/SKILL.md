---
name: laravel
description: Laravel project conventions.
internal: true
---
# Laravel
- Use Artisan generators (`php artisan make:model/controller/migration`). Routes in `routes/web.php`/`api.php`; controllers in `app/Http/Controllers`.
- Migrations for schema; run `php artisan migrate`. Eloquent models for data. Config via `.env` + `config/`.
- Keep deps in `composer.json`. Follow PSR-4 autoloading.
- Verify: `php artisan route:list` / run `php artisan test`. Use the serve tool for a live check, not a blocking `serve` in shell_run.

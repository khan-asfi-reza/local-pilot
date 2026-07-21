---
name: php
description: PHP project conventions.
internal: true
---
# PHP
- Target modern PHP (8+). Use Composer for deps (`composer.json`); autoload via PSR-4.
- Use strict types (`declare(strict_types=1);`) and typed signatures.
- Verify: `php -l <file>` (lint) and run the project's test command if present.

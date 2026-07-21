---
name: ruby
description: Ruby project conventions.
internal: true
---
# Ruby
- Manage deps in a `Gemfile`; install with `bundle install`.
- Follow standard style (2-space indent, snake_case). Prefer small methods.
- Verify: `ruby -c <file>` (syntax), then `bundle exec rspec` / `rake test` if defined.

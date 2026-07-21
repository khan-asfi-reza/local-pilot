---
name: rails
description: Ruby on Rails conventions.
internal: true
---
# Ruby on Rails
- MVC + convention over configuration. Generate with `bin/rails generate`. Routes in `config/routes.rb`; controllers in `app/controllers`; models (ActiveRecord) in `app/models`.
- Schema via migrations: `bin/rails db:migrate`. Gems in the `Gemfile`.
- Verify: `bin/rails runner "puts 1"` / run `bin/rails test`. Live check with the serve tool, not a blocking server.

---
name: rust
description: Rust project conventions.
internal: true
---
# Rust
- Manage deps in `Cargo.toml`. Use `Result`/`?` for errors; avoid `unwrap()` in real code.
- Follow ownership/borrow rules; don't fight the borrow checker with clones everywhere.
- Verify: `cargo build` then `cargo test`. Read compiler errors carefully — they name the fix.

---
name: htmx
description: htmx (hypermedia) conventions.
internal: true
---
# htmx
- Drive interactivity with attributes: `hx-get/post`, `hx-target`, `hx-swap`, `hx-trigger`. The server returns HTML fragments, not JSON.
- Keep logic server-side; htmx swaps the returned markup into the target. Load the script from a CDN or vendored file.
- Verify by serving the page (serve tool) and curling the endpoint that returns the fragment.

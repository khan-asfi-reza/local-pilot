# PollWave — Live Polling Platform (PRD)
Stack: Flask (Python) backend, Vue 3 + Vite frontend, PostgreSQL (SQLAlchemy + migrations).

## 1. Overview
PollWave lets users create polls and everyone vote, with live-updating results. Responsive web.

## 2. Roles & Auth
- Creator: create/close polls, view analytics.
- Voter: vote once per poll.
JWT auth, hashed passwords; voting allowed for authenticated users; one vote per poll enforced.

## 3. Polls & Options
Poll: question, creator, status (open/closed), multi_choice flag, created_at, closes_at. Option: poll, text, votes_count. CRUD polls (creator), add options, close a poll.

## 4. Voting
Vote: poll, option, user (unique per user+poll unless multi_choice). Voting increments the option's count. Endpoint returns a poll with options + current counts + total votes.

## 5. Real-Time Results
A Server-Sent Events endpoint streams updated tallies for a poll so the results chart updates live as votes come in.

## 6. UI/UX
Sticky nav, a polls list (open/closed tabs), a create-poll form with dynamic option inputs, a poll detail/vote page, and a live results page with a bar chart that animates as tallies change. Modern responsive Vue UI, loading/empty states. Controls hit the real API.

## 7. Data
users, polls, options, votes. Migrations before boot; unique constraint on (user, poll) for single-choice. Seed a couple of polls with options and some votes.

## 8. Acceptance
- Signup/login works.
- Create a poll with options; vote once (second vote rejected for single-choice); counts update.
- Results page streams live tallies via SSE.
- Flask boots; migrations apply; endpoints return real data; Vue frontend builds and renders it.

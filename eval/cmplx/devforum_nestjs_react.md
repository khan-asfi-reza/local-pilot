# DevForum — Developer Q&A Community (PRD)
Stack: NestJS (Node + TypeScript) backend, React + Vite frontend, PostgreSQL (TypeORM), Redis for caching hot lists.

## 1. Overview
DevForum is a StackOverflow-style Q&A site: ask, answer, vote, tag, and earn reputation. Responsive web.

## 2. Roles & Auth
- User: ask/answer, vote, comment.
- Moderator: edit/close/delete.
JWT auth, bcrypt hashing, guards on write routes.

## 3. Questions & Answers
Question: title, body (markdown), author, tags[], votes, views, accepted_answer. Answer: question, body, author, votes, accepted flag. CRUD; asker can accept one answer.

## 4. Voting & Reputation
Users upvote/downvote questions and answers (one vote each, toggle). Reputation is derived: +10 per upvote received, +15 for an accepted answer. Endpoint returns a user profile with reputation.

## 5. Tags & Search
Tags with name + description. List questions filtered by tag, sorted by newest/votes/unanswered. Text search over title/body. Cache the hot-questions list in Redis.

## 6. UI/UX
Sticky nav with search, question list with tag filters + sort tabs, a question detail page (body, answers, vote buttons, accept), an ask-question form with tag input, and a user profile page. Modern responsive design, code-friendly typography, loading/empty states. Controls hit the real API.

## 7. Data
users, questions, answers, tags, question_tags, votes. TypeORM entities + migrations. Seed a few questions/answers/tags/votes.

## 8. Acceptance
- Signup/login works.
- Ask a question, post an answer, upvote, accept an answer — all via the API and reflected in the UI and reputation.
- Tag filter + sort work; hot list served (Redis).
- NestJS builds; server boots; endpoints return real data; frontend builds and renders it.

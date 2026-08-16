Product Requirements Document — TaskFlow
A team project-management SaaS (web, responsive).

1. Overview
TaskFlow lets teams organize work into projects, boards, and tasks with real-time
updates. Node.js + TypeScript backend, React + Vite frontend, PostgreSQL, Redis.

2. User Roles
- Owner: create org, invite members, manage billing.
- Member: create/edit projects and tasks, comment.
- Viewer: read-only.

3. Authentication
Email + password signup/login with hashed passwords (bcrypt) and JWT access +
refresh tokens. Protected routes require a valid token; role checks on write ops.

4. Projects & Boards
Each project has a name, description, color, and owner. A project has ordered
boards (columns) like To Do / In Progress / Done. CRUD for projects and boards.

5. Tasks
A task has title, description, status (board), priority (low/med/high/urgent),
assignee, due date, and labels. Endpoints to create, update, move between boards,
assign, and delete. List tasks by project with filters (status, assignee, priority).

6. Comments & Activity
Tasks have threaded comments (author, body, created_at). Every task change writes an
activity record. Endpoint to fetch a task's comments and activity feed.

7. Real-Time
A Server-Sent Events stream pushes task create/move/comment events to connected
clients so boards update live without polling.

8. UI/UX
Responsive board view (drag-free is fine: move via a status dropdown), a sticky
top nav, a project sidebar, task detail drawer, and a create-task modal. Clean
modern design, one accent color, cards with soft shadows, loading + empty states.
Every button and form must call the real API and reflect the result.

9. Data (PostgreSQL)
Tables: users, orgs, memberships, projects, boards, tasks, comments, activity.
Sensible foreign keys and indexes. Seed a demo org with a couple of projects and
tasks so the UI shows real data.

10. Acceptance
- Signup/login works and returns tokens.
- CRUD for projects, boards, tasks works via the API and shows in the UI.
- Moving a task changes its board and persists.
- Comments post and appear.
- The SSE stream delivers updates.
- Backend boots, migrations apply, frontend builds and renders real data.

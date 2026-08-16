# LearnHub — Online Learning Platform (PRD)
Stack: Django + Django REST Framework backend, React + Vite frontend, PostgreSQL.

## 1. Overview
LearnHub lets instructors publish courses and students enroll, learn, and take quizzes with progress tracking. Responsive web.

## 2. Roles & Auth
- Instructor: create courses, lessons, quizzes.
- Student: enroll, watch lessons, take quizzes, track progress.
- Admin: moderate.
Token auth (DRF token or JWT). Passwords hashed. Protected endpoints require auth; write ops role-checked.

## 3. Courses & Lessons
Course: title, slug, description, category, level (beginner/intermediate/advanced), instructor, published flag, price (0 = free). Lesson: belongs to a course, ordered, title, video_url, content (rich text), duration. CRUD for courses and lessons.

## 4. Enrollment & Progress
A student enrolls in a course. Track completed lessons per enrollment and a percent-complete. Endpoints: enroll, mark-lesson-complete, list my enrollments with progress.

## 5. Quizzes
Each course has quizzes; a quiz has questions (multiple choice, one correct option). Submitting answers scores the attempt and stores the result. Endpoint to fetch a quiz, submit answers, and see the score + which were wrong.

## 6. UI/UX
Sticky nav, course catalog grid with filters (category, level, price), course detail with lesson list + progress bar, a lesson player page, a quiz page, and an instructor dashboard to create content. Clean modern design, one accent color, cards, loading/empty states. Every control calls the real API.

## 7. Data (PostgreSQL via Django models + migrations)
users, courses, lessons, enrollments, lesson_completions, quizzes, questions, options, quiz_attempts. FKs + indexes. Seed a couple of courses with lessons and a quiz so the UI shows real data.

## 8. Acceptance
- Signup/login works.
- Instructor creates a course + lessons + a quiz via the API and they appear in the UI.
- Student enrolls, marks lessons complete (progress updates), takes a quiz and gets a score.
- Django migrations apply; server boots; DRF endpoints return real data; frontend builds and renders it.

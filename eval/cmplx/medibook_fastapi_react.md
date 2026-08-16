# MediBook — Clinic Appointment System (PRD)
Stack: FastAPI (Python) backend, React + Vite frontend, PostgreSQL, SQLAlchemy + Alembic migrations.

## 1. Overview
MediBook lets patients book appointments with doctors across available time slots, and doctors manage their schedule. Responsive web.

## 2. Roles & Auth
- Patient: browse doctors, book/cancel appointments, view history.
- Doctor: set availability, view/confirm appointments, write visit notes.
JWT auth (access + refresh), bcrypt password hashing. Protected + role-checked routes.

## 3. Doctors & Availability
Doctor: name, specialty, bio, photo_url, consultation_fee. Availability: doctor, weekday, start_time, end_time, slot_minutes. Generate bookable slots from availability minus already-booked ones.

## 4. Appointments
Appointment: patient, doctor, slot datetime, status (booked/confirmed/completed/cancelled), reason. Booking validates the slot is free and in the future. Endpoints: list doctors (filter by specialty), get a doctor's open slots, book, cancel, doctor-confirm, list my appointments.

## 5. Visit Records
After completion the doctor adds a record: diagnosis, prescription, notes, linked to the appointment. Patient can view their records.

## 6. UI/UX
Sticky nav, doctor directory grid with specialty filter + search, doctor detail with a slot picker calendar, a booking confirmation, patient "my appointments" list with cancel, and a doctor dashboard. Modern responsive design, cards, loading/empty/error states. Controls hit the real API.

## 7. Data
users, doctors, availabilities, appointments, visit_records. Alembic migrations. Seed a few doctors with availability + a sample appointment.

## 8. Acceptance
- Signup/login works.
- Patient sees doctors, opens slots, books; slot becomes unavailable; double-book is rejected.
- Doctor confirms + completes + adds a record; patient sees it.
- Alembic migrations apply; uvicorn boots; endpoints return real data; frontend builds and renders it.

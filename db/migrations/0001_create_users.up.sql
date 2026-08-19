create table if not exists users (
    id            uuid         primary key,
    email         varchar(254) not null,
    name          varchar(100) not null,
    password_hash varchar(72)  not null,
    created_at    timestamptz  not null default now(),
    updated_at    timestamptz  not null default now()
);

-- Emails are stored lower-cased by the application, so a plain unique index is
-- both the uniqueness guard and the login lookup index.
create unique index if not exists users_email_key on users (email);

-- This table holds password hashes and must never be reachable through
-- Supabase's Data API. RLS with no policies denies anon/authenticated every
-- row; the REVOKE denies them the table outright. The backend connects as the
-- owner role, which bypasses RLS, so it is unaffected.
-- Note: no FORCE ROW LEVEL SECURITY -- that would lock the backend out too.
alter table users enable row level security;
revoke all on table users from anon, authenticated;

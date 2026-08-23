create table if not exists machines (
    id          uuid         primary key,
    user_id     uuid         not null references users (id) on delete cascade,
    label       varchar(100) not null,
    location    varchar(150),
    description text,
    created_at  timestamptz  not null default now(),
    updated_at  timestamptz  not null default now(),
    deleted_at  timestamptz
);

-- Partial so a deleted machine's label becomes available again.
create unique index if not exists machines_user_label_key
    on machines (user_id, lower(label))
    where deleted_at is null;

create index if not exists machines_user_id_idx
    on machines (user_id)
    where deleted_at is null;

-- Same reasoning as 0001_create_users: tables in public are otherwise reachable
-- through the Supabase Data API. No FORCE -- that would lock out the backend,
-- which connects as the owner role.
alter table machines enable row level security;
revoke all on table machines from anon, authenticated;

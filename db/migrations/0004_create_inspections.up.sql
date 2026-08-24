create table if not exists inspections (
    id                 uuid        primary key,
    machine_id         uuid        not null references machines (id)  on delete cascade,
    baseline_id        uuid        not null references baselines (id) on delete cascade,
    status             varchar(20) not null,
    z_score            double precision,
    health_score       double precision,
    dominant_indicator varchar(32),
    reason             text,
    audio_path         text,
    inspected_at       timestamptz not null,
    created_at         timestamptz not null default now(),

    constraint inspections_status_chk
        check (status in ('NORMAL', 'WARNING', 'CRITICAL', 'KALIBRASI_KURANG')),
    constraint inspections_health_score_chk
        check (health_score is null or health_score between 0 and 100),
    -- HealthCard derives health_score from z_score, so one is null exactly when
    -- the other is.
    constraint inspections_z_health_chk
        check ((z_score is null) = (health_score is null))
);

-- The only genuinely hot query: the per-machine trend chart.
create index if not exists inspections_machine_time_idx
    on inspections (machine_id, inspected_at desc);
create index if not exists inspections_baseline_id_idx
    on inspections (baseline_id);

alter table inspections enable row level security;
revoke all on table inspections from anon, authenticated;

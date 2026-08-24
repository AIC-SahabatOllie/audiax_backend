create table if not exists baselines (
    id                     uuid         primary key,
    machine_id             uuid         not null references machines (id) on delete cascade,
    schema_version         varchar(16)  not null,
    model_fingerprint      varchar(32)  not null,
    n_windows              integer      not null,
    embedding_dim          integer      not null,
    embedding_dtype        varchar(16)  not null,
    embeddings             bytea        not null,
    backend_stats          jsonb        not null,
    notes                  jsonb        not null default '{}',
    calibration_quality    varchar(10)  not null,
    calibration_audio_path text,
    is_active              boolean      not null default false,
    calibrated_at          timestamptz  not null,
    created_at             timestamptz  not null default now(),

    constraint baselines_quality_chk
        check (calibration_quality in ('baik', 'rendah')),
    constraint baselines_dtype_chk
        check (embedding_dtype = 'float16'),
    constraint baselines_n_windows_chk
        check (n_windows >= 2),
    -- Mirrors the validation in MachineBaseline.from_json_dict so a row whose
    -- byte length disagrees with its declared shape can never be stored.
    -- 2 = bytes per float16.
    constraint baselines_embedding_size_chk
        check (octet_length(embeddings) = n_windows * embedding_dim * 2)
);

-- Makes "exactly one active baseline per machine" impossible to violate, even
-- if two calibrations race in the use case layer.
create unique index if not exists baselines_one_active_per_machine
    on baselines (machine_id)
    where is_active;

create index if not exists baselines_machine_id_idx
    on baselines (machine_id);

-- Answers "which baselines died when the model was redeployed" in one query.
create index if not exists baselines_fingerprint_idx
    on baselines (model_fingerprint);

-- Same reasoning as 0001_create_users: tables in public are otherwise reachable
-- through the Supabase Data API. No FORCE -- that would lock out the backend,
-- which connects as the owner role.
alter table baselines enable row level security;
revoke all on table baselines from anon, authenticated;

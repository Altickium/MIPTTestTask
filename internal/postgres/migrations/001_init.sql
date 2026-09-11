CREATE TABLE polls (
    id UUID PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE,
    question TEXT NOT NULL CHECK (char_length(question) BETWEEN 1 AND 500),
    poll_type TEXT NOT NULL CHECK (poll_type IN ('single_choice', 'multiple_choice')),
    max_choices SMALLINT NOT NULL CHECK (max_choices BETWEEN 1 AND 20),
    vote_buckets SMALLINT NOT NULL CHECK (vote_buckets BETWEEN 1 AND 4096),
    status TEXT NOT NULL CHECK (status IN ('draft', 'active', 'closed')),
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    published_at TIMESTAMPTZ,
    closed_at TIMESTAMPTZ,
    CHECK (starts_at < ends_at)
);

CREATE TABLE poll_options (
    id UUID PRIMARY KEY,
    poll_id UUID NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
    text TEXT NOT NULL CHECK (char_length(text) BETWEEN 1 AND 200),
    position SMALLINT NOT NULL CHECK (position >= 0),
    UNIQUE (poll_id, position)
);

CREATE TABLE poll_runtime_results (
    poll_id UUID PRIMARY KEY REFERENCES polls(id) ON DELETE CASCADE,
    participants_count BIGINT NOT NULL CHECK (participants_count >= 0),
    captured_at TIMESTAMPTZ NOT NULL,
    finalized_at TIMESTAMPTZ
);

CREATE TABLE poll_option_results (
    poll_id UUID NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
    option_id UUID NOT NULL REFERENCES poll_options(id) ON DELETE CASCADE,
    vote_count BIGINT NOT NULL CHECK (vote_count >= 0),
    captured_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (poll_id, option_id)
);

CREATE INDEX polls_status_time_idx ON polls(status, starts_at, ends_at);

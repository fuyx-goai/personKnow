CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS schema_migrations (
    version bigint PRIMARY KEY,
    name varchar(160) NOT NULL,
    applied_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id uuid PRIMARY KEY,
    nickname varchar(80) NOT NULL DEFAULT '微信用户',
    avatar_url varchar(1024),
    status varchar(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    last_login_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE INDEX idx_users_status ON users(status) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_deleted_at ON users(deleted_at);

CREATE TABLE wechat_identities (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    app_id varchar(64) NOT NULL,
    openid_ciphertext bytea NOT NULL,
    openid_hash char(64) NOT NULL,
    unionid_ciphertext bytea,
    unionid_hash char(64),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_wechat_app_openid UNIQUE(app_id, openid_hash)
);
CREATE INDEX idx_wechat_user_id ON wechat_identities(user_id);
CREATE INDEX idx_wechat_unionid_hash ON wechat_identities(unionid_hash) WHERE unionid_hash IS NOT NULL;

CREATE TABLE plans (
    id uuid PRIMARY KEY,
    code varchar(40) NOT NULL UNIQUE,
    name varchar(80) NOT NULL,
    storage_quota_bytes bigint NOT NULL CHECK (storage_quota_bytes >= 0),
    monthly_token_quota bigint NOT NULL CHECK (monthly_token_quota >= 0),
    status varchar(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO plans(id, code, name, storage_quota_bytes, monthly_token_quota)
VALUES ('00000000-0000-7000-8000-000000000001', 'personal_pro', 'Personal Pro', 5368709120, 2000000)
ON CONFLICT (code) DO NOTHING;

CREATE TABLE user_plans (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    plan_id uuid NOT NULL REFERENCES plans(id) ON DELETE RESTRICT,
    starts_at timestamptz NOT NULL DEFAULT now(),
    ends_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_user_plans_user ON user_plans(user_id, starts_at DESC);
CREATE UNIQUE INDEX uk_user_active_plan ON user_plans(user_id) WHERE ends_at IS NULL;

CREATE TABLE auth_sessions (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    refresh_token_hash char(64) NOT NULL UNIQUE,
    client_type varchar(20) NOT NULL CHECK (client_type IN ('miniprogram', 'web')),
    device_label varchar(120),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    last_used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_auth_sessions_user ON auth_sessions(user_id, created_at DESC);
CREATE INDEX idx_auth_sessions_active ON auth_sessions(expires_at) WHERE revoked_at IS NULL;

CREATE TABLE web_login_tickets (
    id uuid PRIMARY KEY,
    secret_hash char(64) NOT NULL UNIQUE,
    status varchar(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'confirmed', 'consumed', 'expired')),
    confirmed_user_id uuid REFERENCES users(id) ON DELETE RESTRICT,
    expires_at timestamptz NOT NULL,
    confirmed_at timestamptz,
    consumed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_web_tickets_status_expiry ON web_login_tickets(status, expires_at);

CREATE TABLE knowledge_bases (
    id uuid PRIMARY KEY,
    owner_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    name varchar(120) NOT NULL,
    category varchar(40) NOT NULL DEFAULT 'uncategorized',
    description varchar(1000) NOT NULL DEFAULT '',
    visibility varchar(20) NOT NULL DEFAULT 'private' CHECK (visibility IN ('private', 'public')),
    status varchar(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'deleting', 'delete_failed')),
    document_count integer NOT NULL DEFAULT 0 CHECK (document_count >= 0),
    chunk_count bigint NOT NULL DEFAULT 0 CHECK (chunk_count >= 0),
    storage_bytes bigint NOT NULL DEFAULT 0 CHECK (storage_bytes >= 0),
    indexed_tokens bigint NOT NULL DEFAULT 0 CHECK (indexed_tokens >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE INDEX idx_libraries_owner_status ON knowledge_bases(owner_user_id, status, updated_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_libraries_public ON knowledge_bases(visibility, status, updated_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_libraries_name_trgm ON knowledge_bases USING gin(name gin_trgm_ops);

CREATE TABLE library_retrieval_settings (
    library_id uuid PRIMARY KEY REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    chunk_size integer NOT NULL DEFAULT 800 CHECK (chunk_size BETWEEN 200 AND 2000),
    chunk_overlap integer NOT NULL DEFAULT 100 CHECK (chunk_overlap BETWEEN 0 AND 500),
    top_k smallint NOT NULL DEFAULT 5 CHECK (top_k BETWEEN 1 AND 20),
    similarity_threshold numeric(5,4) NOT NULL DEFAULT 0.3000 CHECK (similarity_threshold BETWEEN 0 AND 1),
    updated_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (chunk_overlap < chunk_size)
);

CREATE TABLE documents (
    id uuid PRIMARY KEY,
    library_id uuid NOT NULL REFERENCES knowledge_bases(id) ON DELETE RESTRICT,
    uploaded_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    original_name varchar(255) NOT NULL,
    display_name varchar(255) NOT NULL,
    extension varchar(16) NOT NULL,
    mime_type varchar(120) NOT NULL,
    original_bytes bigint NOT NULL CHECK (original_bytes >= 0),
    original_sha256 char(64) NOT NULL,
    original_path varchar(1024) NOT NULL,
    status varchar(30) NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'processing', 'ready', 'failed', 'deleting', 'delete_failed')),
    summary varchar(1000) NOT NULL DEFAULT '',
    tags jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(tags) = 'array'),
    active_content_version_id uuid,
    chunk_count integer NOT NULL DEFAULT 0 CHECK (chunk_count >= 0),
    indexed_tokens bigint NOT NULL DEFAULT 0 CHECK (indexed_tokens >= 0),
    last_indexed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE INDEX idx_documents_library_status ON documents(library_id, status, updated_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_documents_original_hash ON documents(original_sha256);
CREATE INDEX idx_documents_name_trgm ON documents USING gin(display_name gin_trgm_ops);
CREATE INDEX gin_documents_tags ON documents USING gin(tags);

CREATE TABLE document_contents (
    id uuid PRIMARY KEY,
    document_id uuid NOT NULL REFERENCES documents(id) ON DELETE RESTRICT,
    version integer NOT NULL CHECK (version > 0),
    source_type varchar(20) NOT NULL CHECK (source_type IN ('parsed', 'edited')),
    content_path varchar(1024) NOT NULL,
    content_hash char(64) NOT NULL,
    text_bytes bigint NOT NULL CHECK (text_bytes >= 0),
    token_estimate bigint NOT NULL DEFAULT 0 CHECK (token_estimate >= 0),
    structure_map jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_document_content_version UNIQUE(document_id, version)
);
CREATE INDEX idx_content_document_created ON document_contents(document_id, created_at DESC);
CREATE INDEX idx_content_hash ON document_contents(content_hash);
ALTER TABLE documents ADD CONSTRAINT fk_documents_active_content
    FOREIGN KEY (active_content_version_id) REFERENCES document_contents(id) ON DELETE RESTRICT;

CREATE TABLE index_jobs (
    id uuid PRIMARY KEY,
    document_id uuid NOT NULL REFERENCES documents(id) ON DELETE RESTRICT,
    content_version_id uuid REFERENCES document_contents(id) ON DELETE RESTRICT,
    job_type varchar(20) NOT NULL CHECK (job_type IN ('index', 'reindex', 'delete')),
    status varchar(20) NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'ready', 'failed', 'cancelled')),
    progress smallint NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
    stage varchar(40) NOT NULL DEFAULT 'queued',
    attempt_count smallint NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    max_attempts smallint NOT NULL DEFAULT 3 CHECK (max_attempts BETWEEN 1 AND 10),
    next_run_at timestamptz NOT NULL DEFAULT now(),
    lease_expires_at timestamptz,
    worker_id varchar(80),
    error_code varchar(80),
    error_message varchar(1000),
    started_at timestamptz,
    finished_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_jobs_claim ON index_jobs(status, next_run_at, created_at);
CREATE INDEX idx_jobs_lease ON index_jobs(lease_expires_at) WHERE status = 'running';
CREATE UNIQUE INDEX uk_jobs_active_document ON index_jobs(document_id) WHERE status IN ('queued', 'running');

CREATE TABLE chat_sessions (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    scope_type varchar(20) NOT NULL CHECK (scope_type IN ('single_library', 'global')),
    library_id uuid REFERENCES knowledge_bases(id) ON DELETE RESTRICT,
    title varchar(160) NOT NULL DEFAULT '新建问答',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    CHECK ((scope_type = 'single_library' AND library_id IS NOT NULL) OR (scope_type = 'global' AND library_id IS NULL))
);
CREATE INDEX idx_chat_sessions_user ON chat_sessions(user_id, updated_at DESC, id) WHERE deleted_at IS NULL;

CREATE TABLE chat_messages (
    id uuid PRIMARY KEY,
    session_id uuid NOT NULL REFERENCES chat_sessions(id) ON DELETE RESTRICT,
    role varchar(20) NOT NULL CHECK (role IN ('user', 'assistant')),
    content text NOT NULL,
    status varchar(20) NOT NULL DEFAULT 'complete' CHECK (status IN ('streaming', 'complete', 'interrupted', 'failed')),
    model varchar(120),
    input_tokens bigint NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens bigint NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    total_tokens bigint NOT NULL DEFAULT 0 CHECK (total_tokens >= 0),
    request_id varchar(80),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_messages_session_created ON chat_messages(session_id, created_at, id);
CREATE INDEX idx_messages_request_id ON chat_messages(request_id) WHERE request_id IS NOT NULL;

CREATE TABLE message_references (
    id uuid PRIMARY KEY,
    message_id uuid NOT NULL REFERENCES chat_messages(id) ON DELETE RESTRICT,
    document_id uuid NOT NULL REFERENCES documents(id) ON DELETE RESTRICT,
    content_version_id uuid NOT NULL REFERENCES document_contents(id) ON DELETE RESTRICT,
    chunk_id varchar(160) NOT NULL,
    source_name_snapshot varchar(255) NOT NULL,
    location_label varchar(255) NOT NULL DEFAULT '',
    excerpt text NOT NULL,
    similarity numeric(6,5) NOT NULL CHECK (similarity BETWEEN 0 AND 1),
    rank smallint NOT NULL CHECK (rank > 0),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_references_message_rank ON message_references(message_id, rank);
CREATE INDEX idx_references_document ON message_references(document_id);

CREATE TABLE usage_records (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    usage_type varchar(30) NOT NULL CHECK (usage_type IN ('storage', 'embedding', 'input', 'output')),
    quantity bigint NOT NULL,
    unit varchar(20) NOT NULL CHECK (unit IN ('bytes', 'tokens')),
    resource_type varchar(40) NOT NULL,
    resource_id uuid NOT NULL,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_usage_user_month ON usage_records(user_id, occurred_at);
CREATE INDEX idx_usage_resource ON usage_records(resource_type, resource_id);

CREATE TABLE monthly_usage (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    month_start date NOT NULL,
    storage_bytes bigint NOT NULL DEFAULT 0 CHECK (storage_bytes >= 0),
    embedding_tokens bigint NOT NULL DEFAULT 0 CHECK (embedding_tokens >= 0),
    input_tokens bigint NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens bigint NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    total_tokens bigint NOT NULL DEFAULT 0 CHECK (total_tokens >= 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(user_id, month_start)
);

CREATE TABLE audit_logs (
    id bigserial PRIMARY KEY,
    actor_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    action varchar(80) NOT NULL,
    resource_type varchar(40) NOT NULL,
    resource_id uuid,
    result varchar(20) NOT NULL CHECK (result IN ('success', 'failure')),
    request_id varchar(80),
    client_type varchar(20) CHECK (client_type IN ('web', 'miniprogram', 'worker')),
    ip_hash char(64),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_actor_time ON audit_logs(actor_user_id, occurred_at DESC);
CREATE INDEX idx_audit_resource ON audit_logs(resource_type, resource_id);
CREATE INDEX idx_audit_request ON audit_logs(request_id) WHERE request_id IS NOT NULL;

CREATE TABLE idempotency_records (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    route varchar(160) NOT NULL,
    request_key varchar(120) NOT NULL,
    request_hash char(64) NOT NULL,
    response_status integer NOT NULL CHECK (response_status BETWEEN 100 AND 599),
    response_body jsonb NOT NULL DEFAULT '{}'::jsonb,
    resource_id uuid,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_idempotency_user_route_key UNIQUE(user_id, route, request_key)
);
CREATE INDEX idx_idempotency_expiry ON idempotency_records(expires_at);

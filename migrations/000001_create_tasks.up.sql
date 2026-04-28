CREATE TYPE task_state AS ENUM ('received', 'processing', 'done');

CREATE TABLE tasks (
    id              BIGSERIAL PRIMARY KEY,
    type            INTEGER NOT NULL CHECK (type >= 0 AND type <= 9),
    value           INTEGER NOT NULL CHECK (value >= 0 AND value <= 99),
    state           task_state NOT NULL DEFAULT 'received',
    creation_time   DOUBLE PRECISION NOT NULL,
    last_update_time DOUBLE PRECISION NOT NULL
);

CREATE INDEX idx_tasks_state ON tasks (state);
CREATE INDEX idx_tasks_type ON tasks (type);

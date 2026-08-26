ALTER TABLE users ADD COLUMN deleted_at TIMESTAMPTZ;
ALTER TABLE circles ADD COLUMN deleted_at TIMESTAMPTZ;
ALTER TABLE communities ADD COLUMN deleted_at TIMESTAMPTZ;

CREATE INDEX idx_users_deleted_at ON users(deleted_at);
CREATE INDEX idx_circles_deleted_at ON circles(deleted_at);
CREATE INDEX idx_communities_deleted_at ON communities(deleted_at);

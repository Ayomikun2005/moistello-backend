DROP INDEX IF EXISTS idx_communities_deleted_at;
DROP INDEX IF EXISTS idx_circles_deleted_at;
DROP INDEX IF EXISTS idx_users_deleted_at;

ALTER TABLE communities DROP COLUMN deleted_at;
ALTER TABLE circles DROP COLUMN deleted_at;
ALTER TABLE users DROP COLUMN deleted_at;

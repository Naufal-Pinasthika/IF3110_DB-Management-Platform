-- Manual development password reset: admin / AdminDemo-2026!
-- Changes only the existing admin password and revokes its sessions.
BEGIN;

UPDATE users SET password_hash = '$2a$12$S95bIZ2dcjmLTsL2ANN2oew8Vh27twindjC7zO5pIsnApHo/BlwR2' WHERE username = 'admin';

UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP
WHERE user_id = (SELECT id FROM users WHERE username = 'admin')
  AND revoked_at IS NULL;

COMMIT;

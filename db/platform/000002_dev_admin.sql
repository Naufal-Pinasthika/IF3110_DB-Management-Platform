-- Development account only. Run manually after the platform schema migration.
-- Username: admin; password: AdminDemo-2026!
-- Existing accounts and passwords are preserved.
INSERT INTO users (username, password_hash, is_platform_admin, is_active)
VALUES ('admin', '$2a$12$S95bIZ2dcjmLTsL2ANN2oew8Vh27twindjC7zO5pIsnApHo/BlwR2', TRUE, TRUE)
ON CONFLICT (username) DO NOTHING;

-- Development account only. Run manually after the platform schema migration.
-- Username: user1, user2, user3, user4, user5
-- Password: user1-12345, user2-12345, user3-12345, user4-12345, user5-12345
-- Existing accounts and passwords are preserved.
INSERT INTO users (username, password_hash, is_platform_admin, is_active)
VALUES
    ('user1', '$2a$12$80IP/x3TO.gQ4tD0gBZ2quZn/UlHdLWJpDullzy0fLBG27.a/RmAm', FALSE, TRUE),
    ('user2', '$2a$12$AlK32PVzdxCOvAvq6lfnUeyujDHVKNuDP/Ty2bu8gYk145slq1tb2', FALSE, TRUE),
    ('user3', '$2a$12$8rl3QN63bH6bE3rAQX1kXeQ/WHqFfTXedwtQBpgS7s8ok9fuSkhoq', FALSE, TRUE),
    ('user4', '$2a$12$Ji9NjPNJlW0v9Fpd.MfTV.mOxBcC0WzDAcEzZu610VIMn5IQNLHOO', FALSE, TRUE),
    ('user5', '$2a$12$39omZ1fwCIRajiYaCCiIDuR5MJ6HFVUb88alNpvWFE.gpVsPmU/SS', FALSE, TRUE)
ON CONFLICT (username) DO NOTHING;
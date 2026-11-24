ALTER TABLE users ADD COLUMN role ENUM('user', 'super') NOT NULL DEFAULT 'user' AFTER password;

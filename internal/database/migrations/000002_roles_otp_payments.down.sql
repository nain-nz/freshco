DROP TABLE IF EXISTS payments;
DROP TABLE IF EXISTS plans;
DROP TABLE IF EXISTS otp_codes;
ALTER TABLE users
    DROP COLUMN token_version,
    DROP COLUMN email_verified_at,
    DROP COLUMN role;

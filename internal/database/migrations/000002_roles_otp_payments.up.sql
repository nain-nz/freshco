ALTER TABLE users
    ADD COLUMN role              ENUM('user', 'super') NOT NULL DEFAULT 'user' AFTER password,
    ADD COLUMN email_verified_at DATETIME NULL AFTER role,
    ADD COLUMN token_version     INT NOT NULL DEFAULT 0 AFTER email_verified_at;

-- akun yang sudah ada dianggap sudah terverifikasi supaya tidak terkunci
UPDATE users SET email_verified_at = NOW() WHERE email_verified_at IS NULL;

CREATE TABLE otp_codes (
    id         INT AUTO_INCREMENT PRIMARY KEY,
    user_id    INT         NOT NULL,
    purpose    VARCHAR(20) NOT NULL,
    code_hash  CHAR(64)    NOT NULL,
    attempts   TINYINT UNSIGNED NOT NULL DEFAULT 0,
    expires_at DATETIME    NOT NULL,
    used_at    DATETIME    NULL,
    created_at DATETIME    NOT NULL,
    INDEX idx_otp_user_purpose (user_id, purpose, id),
    CONSTRAINT fk_otp_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE plans (
    code   VARCHAR(30)  PRIMARY KEY,
    name   VARCHAR(60)  NOT NULL,
    price  INT UNSIGNED NOT NULL,
    active BOOLEAN      NOT NULL DEFAULT TRUE
);

CREATE TABLE payments (
    id           INT AUTO_INCREMENT PRIMARY KEY,
    order_id     VARCHAR(50)  NOT NULL UNIQUE,
    user_id      INT          NOT NULL,
    plan_code    VARCHAR(30)  NOT NULL,
    amount       INT UNSIGNED NOT NULL,
    status       VARCHAR(20)  NOT NULL DEFAULT 'pending',
    provider     VARCHAR(20)  NOT NULL,
    provider_ref VARCHAR(100) NULL,
    qr_string    TEXT         NULL,
    qr_url       VARCHAR(500) NULL,
    expires_at   DATETIME     NULL,
    paid_at      DATETIME     NULL,
    created_at   DATETIME     NOT NULL,
    INDEX idx_payments_user (user_id, id),
    CONSTRAINT fk_payments_user FOREIGN KEY (user_id)   REFERENCES users(id),
    CONSTRAINT fk_payments_plan FOREIGN KEY (plan_code) REFERENCES plans(code)
);

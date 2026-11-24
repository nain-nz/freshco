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

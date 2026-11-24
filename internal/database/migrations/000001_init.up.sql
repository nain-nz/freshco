CREATE TABLE users (
    id         INT AUTO_INCREMENT PRIMARY KEY,
    username   VARCHAR(50)  NOT NULL UNIQUE,
    email      VARCHAR(100) NOT NULL UNIQUE,
    password   VARCHAR(255) NOT NULL,
    created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE settings (
    `key`      VARCHAR(50)  PRIMARY KEY,
    `value`    VARCHAR(100) NOT NULL,
    updated_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

CREATE TABLE devices (
    id   TINYINT UNSIGNED PRIMARY KEY,
    name VARCHAR(50) NOT NULL
);

CREATE TABLE channels (
    id        TINYINT UNSIGNED PRIMARY KEY,
    device_id TINYINT UNSIGNED NOT NULL,
    name      VARCHAR(30)      NOT NULL UNIQUE,
    CONSTRAINT fk_channels_device FOREIGN KEY (device_id) REFERENCES devices(id)
);

CREATE TABLE readings (
    `time`     DATETIME(3)      NOT NULL,
    channel_id TINYINT UNSIGNED NOT NULL,
    `value`    FLOAT            NOT NULL,
    PRIMARY KEY (`time`, channel_id),
    INDEX idx_readings_channel_time (channel_id, `time`),
    CONSTRAINT fk_readings_channel FOREIGN KEY (channel_id) REFERENCES channels(id)
);

CREATE TABLE alerts (
    id           INT AUTO_INCREMENT PRIMARY KEY,
    `time`       DATETIME         NOT NULL,
    channel_id   TINYINT UNSIGNED NULL,
    type         VARCHAR(30)      NOT NULL,
    `value`      FLOAT            NULL,
    acknowledged BOOLEAN          NOT NULL DEFAULT FALSE,
    INDEX idx_alerts_time (`time`),
    CONSTRAINT fk_alerts_channel FOREIGN KEY (channel_id) REFERENCES channels(id)
);

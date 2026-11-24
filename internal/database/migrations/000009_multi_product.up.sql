-- Multi-produk: tiap box punya ID (serial), dimiliki owner, bisa punya anggota.
-- Pembayaran: order + jadwal tagihan (DP lalu cicilan). Sensor suhu jadi 4 titik.

ALTER TABLE users
    ADD COLUMN role ENUM('user', 'admin') NOT NULL DEFAULT 'user' AFTER password;

CREATE TABLE products (
    id         INT AUTO_INCREMENT PRIMARY KEY,
    serial     VARCHAR(20) NOT NULL UNIQUE,   -- ID publik, dipakai di topic MQTT
    claim_hash CHAR(64)    NOT NULL UNIQUE,   -- hash kode klaim (kode asli tidak disimpan)
    status     ENUM('available', 'pending_payment', 'active', 'suspended') NOT NULL DEFAULT 'available',
    claimed_at DATETIME    NULL,
    created_at DATETIME    NOT NULL
);

CREATE TABLE product_members (
    product_id INT NOT NULL,
    user_id    INT NOT NULL,
    role       ENUM('owner', 'member') NOT NULL,
    joined_at  DATETIME NOT NULL,
    PRIMARY KEY (product_id, user_id),
    INDEX idx_members_user (user_id),
    CONSTRAINT fk_members_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE,
    CONSTRAINT fk_members_user    FOREIGN KEY (user_id)    REFERENCES users(id)    ON DELETE CASCADE
);

CREATE TABLE product_invites (
    id         INT AUTO_INCREMENT PRIMARY KEY,
    product_id INT      NOT NULL,
    code_hash  CHAR(64) NOT NULL UNIQUE,
    expires_at DATETIME NOT NULL,
    used_by    INT      NULL,
    created_at DATETIME NOT NULL,
    INDEX idx_invites_product (product_id),
    CONSTRAINT fk_invites_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE,
    CONSTRAINT fk_invites_user    FOREIGN KEY (used_by)    REFERENCES users(id)    ON DELETE SET NULL
);

CREATE TABLE orders (
    id         INT AUTO_INCREMENT PRIMARY KEY,
    product_id INT              NOT NULL UNIQUE,
    user_id    INT              NOT NULL,
    total      INT UNSIGNED     NOT NULL,
    dp         INT UNSIGNED     NOT NULL,
    tenor      TINYINT UNSIGNED NOT NULL,      -- 1 = bayar langsung; 2/4/6 = jumlah cicilan
    created_at DATETIME         NOT NULL,
    CONSTRAINT fk_orders_product FOREIGN KEY (product_id) REFERENCES products(id),
    CONSTRAINT fk_orders_user    FOREIGN KEY (user_id)    REFERENCES users(id)
);

CREATE TABLE payments (
    id                INT AUTO_INCREMENT PRIMARY KEY,
    order_id          INT              NOT NULL,
    seq               TINYINT UNSIGNED NOT NULL,   -- 0 = DP, 1..n = cicilan / pelunasan
    amount            INT UNSIGNED     NOT NULL,
    due_date          DATE             NOT NULL,
    method            ENUM('midtrans', 'cash') NULL,
    provider_order_id VARCHAR(50)      NULL UNIQUE,
    status            ENUM('pending', 'paid', 'expired', 'failed') NOT NULL DEFAULT 'pending',
    paid_at           DATETIME         NULL,
    confirmed_by      INT              NULL,
    created_at        DATETIME         NOT NULL,
    UNIQUE KEY uq_payments_order_seq (order_id, seq),
    CONSTRAINT fk_payments_order FOREIGN KEY (order_id)     REFERENCES orders(id) ON DELETE CASCADE,
    CONSTRAINT fk_payments_admin FOREIGN KEY (confirmed_by) REFERENCES users(id)  ON DELETE SET NULL
);

-- Data lama (satu box) dipindah ke produk 1. Hanya dibuat kalau sudah ada data sensor/alert;
-- instalasi baru tidak mendapat produk ini. Produk ini belum diklaim: admin menekan
-- "reset kode klaim" untuk mendapat kodenya.
INSERT INTO products (serial, claim_hash, status, claimed_at, created_at)
SELECT 'FC-LEGACY', SHA2('legacy-tidak-bisa-ditebak', 256), 'available', NULL, NOW()
FROM DUAL
WHERE EXISTS (SELECT 1 FROM readings) OR EXISTS (SELECT 1 FROM alerts);

-- Tanpa produk lama, pengaturan global bawaan tidak punya pemilik; hapus saja
-- (pengaturan bawaan dibuat per produk saat admin menambah produk).
DELETE FROM settings WHERE NOT EXISTS (SELECT 1 FROM products);

-- settings: kunci jadi (product_id, key)
ALTER TABLE settings
    ADD COLUMN product_id INT NOT NULL DEFAULT 1 FIRST,
    DROP PRIMARY KEY,
    ADD PRIMARY KEY (product_id, `key`),
    ADD CONSTRAINT fk_settings_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE;
ALTER TABLE settings ALTER COLUMN product_id DROP DEFAULT;

-- sensor suhu: tengah -> kiri (posisi berubah), jadi data channel itu dibuang.
-- Alert suhu tinggi lama (terikat channel tengah) tetap disimpan tanpa channel.
UPDATE alerts SET channel_id = NULL WHERE channel_id = 2;
DELETE FROM readings WHERE channel_id = 2;
UPDATE channels SET name = 'suhu_kiri' WHERE id = 2;
-- channel 8 (suhu_kanan) dibuat oleh: go run . seed

-- readings: kunci jadi (product_id, time, channel_id)
ALTER TABLE readings
    ADD COLUMN product_id INT NOT NULL DEFAULT 1 FIRST,
    DROP PRIMARY KEY,
    ADD PRIMARY KEY (product_id, `time`, channel_id),
    ADD INDEX idx_readings_product_channel_time (product_id, channel_id, `time`),
    ADD CONSTRAINT fk_readings_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE;
ALTER TABLE readings ALTER COLUMN product_id DROP DEFAULT;

-- alerts
ALTER TABLE alerts
    ADD COLUMN product_id INT NOT NULL DEFAULT 1 AFTER id,
    ADD INDEX idx_alerts_product_time (product_id, `time`),
    ADD CONSTRAINT fk_alerts_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE;
ALTER TABLE alerts ALTER COLUMN product_id DROP DEFAULT;

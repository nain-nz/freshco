ALTER TABLE alerts
    DROP FOREIGN KEY fk_alerts_product,
    DROP INDEX idx_alerts_product_time,
    DROP COLUMN product_id;

ALTER TABLE readings
    DROP FOREIGN KEY fk_readings_product,
    DROP PRIMARY KEY,
    DROP INDEX idx_readings_product_channel_time,
    DROP COLUMN product_id,
    ADD PRIMARY KEY (`time`, channel_id);

ALTER TABLE settings
    DROP FOREIGN KEY fk_settings_product,
    DROP PRIMARY KEY,
    DROP COLUMN product_id,
    ADD PRIMARY KEY (`key`);

UPDATE channels SET name = 'suhu_tengah' WHERE id = 2;

DROP TABLE IF EXISTS payments;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS product_invites;
DROP TABLE IF EXISTS product_members;
DROP TABLE IF EXISTS products;

ALTER TABLE users DROP COLUMN role;

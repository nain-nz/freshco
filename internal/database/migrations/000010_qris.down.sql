ALTER TABLE payments
    DROP COLUMN qr_expires_at,
    DROP COLUMN qr_string,
    MODIFY COLUMN method ENUM('midtrans', 'qris', 'cash') NULL;

UPDATE payments SET method = 'midtrans' WHERE method = 'qris';

ALTER TABLE payments
    MODIFY COLUMN method ENUM('midtrans', 'cash') NULL;

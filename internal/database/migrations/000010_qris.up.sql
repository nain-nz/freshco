-- Pembayaran hanya lewat QRIS (atau cash). Metode lama 'midtrans' diganti 'qris', dan QR
-- yang sudah dibuat disimpan supaya tagihan yang sama menampilkan QR yang sama
-- selama belum dibayar dan belum kedaluwarsa.

ALTER TABLE payments
    MODIFY COLUMN method ENUM('midtrans', 'qris', 'cash') NULL;

UPDATE payments SET method = 'qris' WHERE method = 'midtrans';

ALTER TABLE payments
    MODIFY COLUMN method ENUM('qris', 'cash') NULL,
    ADD COLUMN qr_string     TEXT     NULL AFTER provider_order_id,
    ADD COLUMN qr_expires_at DATETIME NULL AFTER qr_string;

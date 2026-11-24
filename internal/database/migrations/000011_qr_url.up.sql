-- URL gambar QR dari penyedia; dipakai hanya di sandbox untuk simulator QRIS.
ALTER TABLE payments ADD COLUMN qr_url VARCHAR(255) NULL AFTER qr_string;

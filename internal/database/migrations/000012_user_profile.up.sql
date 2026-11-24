-- Data diri user: nomor HP, tanggal lahir, alamat. NULL untuk akun lama
-- (admin melihat "Belum diisi"; user bisa melengkapi dari menu akun).
ALTER TABLE users
    ADD COLUMN phone      VARCHAR(20)  NULL AFTER email,
    ADD COLUMN birth_date DATE         NULL AFTER phone,
    ADD COLUMN address    VARCHAR(255) NULL AFTER birth_date;

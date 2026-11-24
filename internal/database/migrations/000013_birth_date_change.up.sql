-- Kapan tanggal lahir terakhir DIUBAH (bukan diisi pertama kali). Tanggal lahir
-- hanya boleh diubah seminggu sekali; NULL = belum pernah diubah.
ALTER TABLE users
    ADD COLUMN birth_date_changed_at DATETIME NULL AFTER birth_date;

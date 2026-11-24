-- tidak ada lagi Super User: semua user punya akses penuh
ALTER TABLE users DROP COLUMN role;

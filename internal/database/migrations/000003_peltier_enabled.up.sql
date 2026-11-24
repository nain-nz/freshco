-- mode auto/manual diganti saklar peltier_enabled (true = otomatis ikut target suhu)
DELETE FROM settings WHERE `key` = 'mode';
INSERT IGNORE INTO settings (`key`, `value`) VALUES ('peltier_enabled', 'true');

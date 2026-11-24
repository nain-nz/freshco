DELETE FROM settings WHERE `key` = 'peltier_enabled';
INSERT IGNORE INTO settings (`key`, `value`) VALUES ('mode', 'auto');

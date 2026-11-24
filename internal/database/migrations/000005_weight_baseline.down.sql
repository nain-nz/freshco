DELETE FROM settings WHERE `key` IN ('weight_baseline_g', 'weight_tolerance_g');
INSERT IGNORE INTO settings (`key`, `value`) VALUES ('weight_alert_g', '100');

-- batas berat minimum diganti berat acuan + toleransi (acuan 0 = belum diatur)
DELETE FROM settings WHERE `key` = 'weight_alert_g';
INSERT IGNORE INTO settings (`key`, `value`) VALUES
    ('weight_baseline_g', '0'),
    ('weight_tolerance_g', '50');

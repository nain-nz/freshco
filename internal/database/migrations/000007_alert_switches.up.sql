-- acuan berat dihapus (dibandingkan dengan data sensor 1 menit terakhir);
-- tambah saklar per jenis alert dan toleransi suhu
DELETE FROM settings WHERE `key` = 'weight_baseline_g';
INSERT IGNORE INTO settings (`key`, `value`) VALUES
    ('temp_tolerance_c', '3'),
    ('alert_temp_enabled', 'true'),
    ('alert_gas_enabled', 'true'),
    ('alert_weight_down_enabled', 'true'),
    ('alert_weight_up_enabled', 'true');

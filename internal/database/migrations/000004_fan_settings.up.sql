-- pengaturan fan: aktif/nonaktif dan kecepatan (%), diatur user
INSERT IGNORE INTO settings (`key`, `value`) VALUES
    ('fan_enabled', 'true'),
    ('fan_speed', '70');

DELETE FROM settings WHERE `key` IN ('temp_tolerance_c', 'alert_temp_enabled',
    'alert_gas_enabled', 'alert_weight_down_enabled', 'alert_weight_up_enabled');
INSERT IGNORE INTO settings (`key`, `value`) VALUES ('weight_baseline_g', '0');

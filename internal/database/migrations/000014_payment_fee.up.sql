-- Biaya langganan bulanan.
--  orders.subscription_fee : biaya per bulan yang berlaku untuk order itu (0 = tanpa langganan,
--                            termasuk semua order lama sebelum fitur ini).
--  payments.fee            : bagian langganan di dalam `amount`. `amount` tetap TOTAL yang dibayar.
--                            Selama masih ada cicilan, langganan digabung ke tagihan cicilan
--                            (amount = cicilan + fee). Setelah itu, tagihan langganan berdiri
--                            sendiri (amount = fee). 0 untuk DP dan tagihan lama.
ALTER TABLE orders
    ADD COLUMN subscription_fee INT NOT NULL DEFAULT 0 AFTER tenor;
ALTER TABLE payments
    ADD COLUMN fee INT NOT NULL DEFAULT 0 AFTER amount;

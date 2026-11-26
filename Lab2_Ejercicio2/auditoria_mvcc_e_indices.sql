INSERT INTO catalogo_planes_bcnf (plan_id, nombre_plan, tarifa_mensual_centavos, cuota_datos_mb)
VALUES ('PLAN-5G-TEST', 'Plan 5G Prueba', 2000, 51200)
ON CONFLICT (plan_id) DO NOTHING;

WITH abonado_base AS (
    INSERT INTO abonados_bcnf (msisdn, plan_id, saldo_centavos)
    VALUES ('+584145550101', 'PLAN-5G-TEST', 500000)
    ON CONFLICT (msisdn) DO UPDATE SET saldo_centavos = EXCLUDED.saldo_centavos
    RETURNING abonado_id
)
INSERT INTO transacciones_bss (
    idempotency_key,
    abonado_id,
    monto_centavos,
    tipo_operacion,
    metadata_jsonb,
    created_at
)
SELECT
    'idem-tx-' || g::text,
    (SELECT abonado_id FROM abonado_base),
    100 + (g % 500),
    'CONSUMO_DATOS',
    jsonb_build_object(
        'antena_5g', CASE WHEN g % 250 = 0 THEN 'CCS-NORTE-01' ELSE 'CCS-SUR-0' || (g % 9 + 1)::text END,
        'roaming', (g % 250 = 0),
        'latencia_ms', 10 + (g % 40)
    ),
    NOW() - ((50000 - g) || ' seconds')::interval
FROM generate_series(1, 50000) AS g
ON CONFLICT (idempotency_key) DO NOTHING;

SELECT ctid, xmin, xmax, msisdn, saldo_centavos
FROM abonados_bcnf
WHERE msisdn = '+584145550101';

UPDATE abonados_bcnf
SET saldo_centavos = saldo_centavos - 500
WHERE msisdn = '+584145550101';

SELECT ctid, xmin, xmax, msisdn, saldo_centavos
FROM abonados_bcnf
WHERE msisdn = '+584145550101';

EXPLAIN (ANALYZE, BUFFERS)
SELECT tx_id, abonado_id, monto_centavos, metadata_jsonb
FROM transacciones_bss
WHERE metadata_jsonb @> '{"antena_5g": "CCS-NORTE-01", "roaming": true}'::jsonb;

CREATE INDEX IF NOT EXISTS idx_tx_metadata_gin
    ON transacciones_bss USING GIN (metadata_jsonb jsonb_path_ops);

CREATE INDEX IF NOT EXISTS idx_tx_created_at_brin
    ON transacciones_bss USING BRIN (created_at);

EXPLAIN (ANALYZE, BUFFERS)
SELECT tx_id, abonado_id, monto_centavos, metadata_jsonb
FROM transacciones_bss
WHERE metadata_jsonb @> '{"antena_5g": "CCS-NORTE-01", "roaming": true}'::jsonb;

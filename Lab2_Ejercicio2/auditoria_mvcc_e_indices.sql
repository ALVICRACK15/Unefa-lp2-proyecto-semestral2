-- lab6_postgresql_bcnf_mvcc_indices/02_auditoria_mvcc_e_indices.sql

-- 1. Cargar datos de prueba (50.000 transacciones + abonado y plan de prueba)
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

-- 2. Auditoría MVCC antes y después de un UPDATE sobre un abonado
SELECT ctid, xmin, xmax, msisdn, saldo_centavos
FROM abonados_bcnf
WHERE msisdn = '+584145550101';

UPDATE abonados_bcnf
SET saldo_centavos = saldo_centavos - 500
WHERE msisdn = '+584145550101';

SELECT ctid, xmin, xmax, msisdn, saldo_centavos
FROM abonados_bcnf
WHERE msisdn = '+584145550101';

-- 3. Plan de ejecución ANTES de crear índices (esperable: Seq Scan)
EXPLAIN (ANALYZE, BUFFERS)
SELECT tx_id, abonado_id, monto_centavos, metadata_jsonb
FROM transacciones_bss
WHERE metadata_jsonb @> '{"antena_5g": "CCS-NORTE-01", "roaming": true}'::jsonb;

-- 4. Creación de índices GIN (jsonb_path_ops) y BRIN (created_at)
CREATE INDEX IF NOT EXISTS idx_tx_metadata_gin
    ON transacciones_bss USING GIN (metadata_jsonb jsonb_path_ops);

CREATE INDEX IF NOT EXISTS idx_tx_created_at_brin
    ON transacciones_bss USING BRIN (created_at);

-- 5. Plan de ejecución DESPUÉS de crear el índice GIN (esperable: Bitmap Index Scan / Bitmap Heap Scan)
EXPLAIN (ANALYZE, BUFFERS)
SELECT tx_id, abonado_id, monto_centavos, metadata_jsonb
FROM transacciones_bss
WHERE metadata_jsonb @> '{"antena_5g": "CCS-NORTE-01", "roaming": true}'::jsonb;
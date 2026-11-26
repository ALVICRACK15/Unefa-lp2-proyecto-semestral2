BEGIN;

CREATE TABLE IF NOT EXISTS catalogo_planes_bcnf (
    plan_id VARCHAR(20) PRIMARY KEY,
    nombre_plan VARCHAR(100) NOT NULL,
    tarifa_mensual_centavos BIGINT NOT NULL CHECK (tarifa_mensual_centavos >= 0),
    cuota_datos_mb INTEGER NOT NULL CHECK (cuota_datos_mb > 0)
);

CREATE TABLE IF NOT EXISTS abonados_bcnf (
    abonado_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    msisdn VARCHAR(16) UNIQUE NOT NULL CHECK (msisdn ~ '^\+[1-9]\d{6,14}$'),
    plan_id VARCHAR(20) NOT NULL REFERENCES catalogo_planes_bcnf(plan_id),
    saldo_centavos BIGINT NOT NULL DEFAULT 0 CHECK (saldo_centavos >= 0),
    activo BOOLEAN NOT NULL DEFAULT TRUE,
    actualizado_en TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS transacciones_bss (
    tx_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key VARCHAR(64) UNIQUE NOT NULL,
    abonado_id UUID NOT NULL REFERENCES abonados_bcnf(abonado_id),
    monto_centavos BIGINT NOT NULL CHECK (monto_centavos > 0),
    tipo_operacion VARCHAR(20) NOT NULL CHECK (tipo_operacion IN ('RECARGA', 'CONSUMO_DATOS', 'REEMBOLSO')),
    metadata_jsonb JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS outbox_events (
    event_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_id UUID NOT NULL,
    event_type VARCHAR(50) NOT NULL,
    payload_jsonb JSONB NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'PROCESSED', 'FAILED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ
);

INSERT INTO catalogo_planes_bcnf (plan_id, nombre_plan, tarifa_mensual_centavos, cuota_datos_mb)
VALUES 
    ('PLAN-5G-BASIC', 'Plan 5G Básico 10GB', 1500, 10240),
    ('PLAN-5G-PRO', 'Plan 5G Ilimitado Pro', 3500, 102400)
ON CONFLICT (plan_id) DO NOTHING;

COMMIT;

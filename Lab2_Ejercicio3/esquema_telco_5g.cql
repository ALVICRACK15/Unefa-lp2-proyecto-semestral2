CREATE KEYSPACE IF NOT EXISTS unefa_telco_5g
WITH replication = {
    'class': 'NetworkTopologyStrategy',
    'replication_factor': 3
};

USE unefa_telco_5g;

CREATE TABLE IF NOT EXISTS cdr_por_abonado_dia (
    msisdn TEXT,
    bucket_fecha TEXT,
    timestamp_evento TIMESTAMP,
    id_cdr UUID,
    tipo_evento TEXT,
    monto_centavos BIGINT,
    celda_5g TEXT,
    PRIMARY KEY ((msisdn, bucket_fecha), timestamp_evento, id_cdr)
) WITH CLUSTERING ORDER BY (timestamp_evento DESC, id_cdr ASC)
  AND compaction = {
      'class': 'TimeWindowCompactionStrategy',
      'compaction_window_unit': 'DAYS',
      'compaction_window_size': 1
  }
  AND default_time_to_live = 2592000;

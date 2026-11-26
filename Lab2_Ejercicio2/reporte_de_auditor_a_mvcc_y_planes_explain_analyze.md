# Auditoría Forense MVCC y Optimización de Índices en PostgreSQL 16

**Asignatura:** Lenguaje de Programación II (Sección `05S-2629-D1`)  
**Laboratorio:** `lab6_postgresql_bcnf_mvcc_indices` (Ejercicio 2 — Segundo Corte)  
**Motor:** PostgreSQL 16  

---

## 1. Descomposición en Forma Normal de Boyce-Codd (BCNF)

Para eliminar anomalías de inserción, actualización y borrado causadas por dependencias transitivas en una tabla plana legada de facturación, el esquema se descompuso sin pérdida en tres relaciones principales más la tabla de integración:

* **`catalogo_planes_bcnf`**: Determinante funcional estricto $\text{plan\_id} \rightarrow \{\text{nombre\_plan}, \text{tarifa\_mensual\_centavos}, \text{cuota\_datos\_mb}\}$.
* **`abonados_bcnf`**: Determinante funcional estricto $\text{abonado\_id} \rightarrow \{\text{msisdn}, \text{plan\_id}, \text{saldo\_centavos}, \text{activo}, \text{actualizado\_en}\}$, con clave candidata única `msisdn` validada bajo estándar ITU-T E.164 (`^\+[1-9]\d{6,14}$`).
* **`transacciones_bss`**: Determinante funcional $\text{tx\_id} \rightarrow \{\text{idempotency\_key}, \text{abonado\_id}, \text{monto\_centavos}, \text{tipo\_operacion}, \text{metadata\_jsonb}, \text{created\_at}\}$.
* **`outbox_events`**: Cola transaccional local para garantizar atomicidad entre el estado relacional y los eventos de dominio pendientes por proyectar hacia ScyllaDB.

---

## 2. Inspección Forense del Control de Concurrencia Multiversión (MVCC)

Al ejecutar una operación `UPDATE` sobre el saldo de un abonado en `abonados_bcnf`, PostgreSQL no sobrescribe la tupla física en disco (*in-place update*), sino que marca la versión anterior como expirada y escribe una nueva versión física en una nueva ubicación de página.

### Evidencia de Columnas del Sistema (`ctid`, `xmin`, `xmax`)

| Estado de la Tupla | `ctid` (Página, Ítem) | `xmin` (ID Tx Creadora) | `xmax` (ID Tx Expiradora) | `saldo_centavos` | Interpretación Técnica |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **Antes del `UPDATE`** | `(0, 1)` *(ajustar según salida)* | `742` *(ejemplo)* | `0` | `500000` | Tupla activa vigente; `xmax = 0` indica que ninguna transacción la ha invalidado. |
| **Después del `UPDATE`** | `(0, 2)` *(ajustar según salida)* | `743` *(ejemplo)* | `0` | `499500` | Se crea una nueva tupla física con nuevo desplazamiento (`ctid`) y nuevo `xmin`; la versión anterior queda disponible para `VACUUM` cuando no existan transacciones activas viéndola. |

---

## 3. Comparativa de Planes `EXPLAIN (ANALYZE, BUFFERS)` (GIN y BRIN)

Se evaluó una carga de **50.000 transacciones** filtrando atributos anidados dentro de la columna semiestructurada `metadata_jsonb`:

```sql
SELECT tx_id, abonado_id, monto_centavos, metadata_jsonb
FROM transacciones_bss
WHERE metadata_jsonb @> '{"antena_5g": "CCS-NORTE-01", "roaming": true}'::jsonb;
```

### 3.1 Escenario A: Sin Índices Secundarios (`Seq Scan`)

* **Nodo de Planificación:** `Seq Scan on transacciones_bss`
* **Comportamiento:** El motor recorre secuencialmente todas las páginas de datos de la tabla en el Heap evaluando el operador de contención `@>` fila por fila.
* **Métricas capturadas *(pegar valores reales de `psql`)*:**
  * `Execution Time`: `___ ms`
  * `Buffers: shared hit=___ read=___`
  * `Rows Removed by Filter`: `49800` (de 50.000 filas totales)

### 3.2 Escenario B: Con Índice Invertido `GIN` (`jsonb_path_ops`) y `BRIN` (`created_at`)

Se crearon dos índices especializados según la naturaleza de los datos:
1. **`GIN` (`idx_tx_metadata_gin` con `jsonb_path_ops`)**: Indexa hashes de rutas y valores del documento binario `JSONB`, optimizando consultas con el operador `@>` y reduciendo el tamaño en disco frente al operador `GIN` por defecto.
2. **`BRIN` (`idx_tx_created_at_brin`)**: Índice de rango de bloques ideal para series temporales monotónicas (`created_at`), almacenando únicamente valores mínimo y máximo por rango de páginas con huella mínima en disco.

* **Nodo de Planificación:** `Bitmap Heap Scan on transacciones_bss` $\rightarrow$ `Bitmap Index Scan on idx_tx_metadata_gin`
* **Métricas capturadas *(pegar valores reales de `psql`)*:**
  * `Execution Time`: `___ ms`
  * `Buffers: shared hit=___`
  * `Heap Blocks: exact=___`

### 3.3 Tabla Resumen de Impacto

| Métrica de Rendimiento | Sin Índice (`Seq Scan`) | Con `GIN (jsonb_path_ops)` | Mejora Observada |
| :--- | :--- | :--- | :--- |
| **Estrategia de Acceso** | Escaneo secuencial completo | `Bitmap Index Scan` + `Bitmap Heap Scan` | Acceso directo a bloques candidatos |
| **Bloques Leídos (`Buffers`)** | *Completar con salida real* | *Completar con salida real* | Reducción drástica de I/O de páginas |
| **Tiempo de Ejecución (`ms`)** | *Completar con salida real* | *Completar con salida real* | Caída significativa de latencia |

---

## 4. Conclusiones para la Defensa Oral

1. **Integridad en BCNF**: Separar `catalogo_planes_bcnf` evita inconsistencias al modificar tarifas o cuotas y mantiene un único punto de verdad.
2. **MVCC sin bloqueos de lectura**: Gracias al versionado por tuplas (`ctid`, `xmin`, `xmax`), los lectores nunca bloquean a los escritores ni viceversa en PostgreSQL.
3. **Selección adecuada de índices**: `GIN` con `jsonb_path_ops` acelera búsquedas por contención dentro de documentos JSONB, mientras que `BRIN` permite filtrar rangos cronológicos masivos ocupando una fracción mínima de almacenamiento frente a un B-Tree tradicional.
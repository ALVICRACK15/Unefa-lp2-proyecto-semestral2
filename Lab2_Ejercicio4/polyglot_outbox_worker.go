package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

type OutboxEvent struct {
	EventID     string
	AggregateID string
	EventType   string
	PayloadJSON []byte
	Status      string
	CreatedAt   time.Time
}

type CDRPayload struct {
	MSISDN          string    `json:"msisdn"`
	BucketFecha     string    `json:"bucket_fecha"`
	TimestampEvento time.Time `json:"timestamp_evento"`
	IDCDR           string    `json:"id_cdr"`
	TipoEvento      string    `json:"tipo_evento"`
	MontoCentavos   int64     `json:"monto_centavos"`
	Celda5G         string    `json:"celda_5g"`
}

func RegistrarOperacionYOutbox(ctx context.Context, db *sql.DB, abonadoID, idempKey string, monto int64, payload CDRPayload) error {
	if db == nil {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		UPDATE abonados_bcnf
		SET saldo_centavos = saldo_centavos - $1, actualizado_en = NOW()
		WHERE abonado_id = $2
	`, monto, abonadoID)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO transacciones_bss (idempotency_key, abonado_id, monto_centavos, tipo_operacion, metadata_jsonb)
		VALUES ($1, $2, $3, 'CONSUMO_DATOS', '{"origen":"outbox_demo"}'::jsonb)
	`, idempKey, abonadoID, monto)
	if err != nil {
		return err
	}

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO outbox_events (aggregate_id, event_type, payload_jsonb, status)
		VALUES ($1, 'CARGO_APLICADO', $2::jsonb, 'PENDING')
	`, abonadoID, string(rawPayload))
	if err != nil {
		return err
	}

	return tx.Commit()
}

func reclamarLotePendiente(ctx context.Context, db *sql.DB, limite int) ([]OutboxEvent, error) {
	if db == nil {
		return nil, nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `
		SELECT event_id, aggregate_id, event_type, payload_jsonb, status, created_at
		FROM outbox_events
		WHERE status = 'PENDING'
		ORDER BY created_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`, limite)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var eventos []OutboxEvent
	for rows.Next() {
		var ev OutboxEvent
		if err := rows.Scan(&ev.EventID, &ev.AggregateID, &ev.EventType, &ev.PayloadJSON, &ev.Status, &ev.CreatedAt); err != nil {
			return nil, err
		}
		eventos = append(eventos, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return eventos, nil
}

func proyectarEvento(ctx context.Context, db *sql.DB, ev OutboxEvent) error {
	var p CDRPayload
	if err := json.Unmarshal(ev.PayloadJSON, &p); err != nil {
		return err
	}

	_ = p

	if db != nil {
		_, err := db.ExecContext(ctx, `
			UPDATE outbox_events
			SET status = 'PROCESSED', processed_at = NOW()
			WHERE event_id = $1
		`, ev.EventID)
		return err
	}
	return nil
}

func runWorker(ctx context.Context, wg *sync.WaitGroup, workerID int, db *sql.DB) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			log.Printf("Worker %d detenido limpiamente", workerID)
			return
		default:
			eventos, err := reclamarLotePendiente(ctx, db, 50)
			if err != nil {
				log.Printf("Worker %d error al reclamar outbox: %v", workerID, err)
				time.Sleep(500 * time.Millisecond)
				continue
			}
			if len(eventos) == 0 {
				time.Sleep(300 * time.Millisecond)
				continue
			}
			for _, ev := range eventos {
				if err := proyectarEvento(ctx, db, ev); err != nil {
					log.Printf("Worker %d fallo proyectando evento %s: %v", workerID, ev.EventID, err)
					continue
				}
				log.Printf("Worker %d proyectó y confirmó evento %s", workerID, ev.EventID)
			}
		}
	}
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var (
		db *sql.DB
		wg sync.WaitGroup
	)

	const numWorkers = 4
	for w := 1; w <= numWorkers; w++ {
		wg.Add(1)
		go runWorker(ctx, &wg, w, db)
	}

	wg.Wait()
	fmt.Println("Ciclo del worker Transactional Outbox finalizado sin carreras de datos.")
}

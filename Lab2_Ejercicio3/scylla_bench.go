package main

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

type CDRRecord struct {
	MSISDN          string
	BucketFecha     string
	TimestampEvento time.Time
	IDCDR           string
	TipoEvento      string
	MontoCentavos   int64
	Celda5G         string
}

func percentil(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil((p/100.0)*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func escribirCDR(rec CDRRecord) error {
	// Aquí va la consulta CQL con gocql (ej. session.Query("INSERT INTO cdr_por_abonado_dia ...").Consistency(gocql.LocalQuorum).Exec())
	// Simulamos una micro-operación mientras conectas el driver real de ScyllaDB:
	time.Sleep(50 * time.Microsecond)
	return nil
}

func main() {
	const (
		workers   = 16
		totalOps  = 4000
		bucketHoy = "2026-11-24"
	)

	jobs := make(chan int, totalOps)
	latenciasUs := make([]float64, 0, totalOps)

	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)

	inicioTotal := time.Now()

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := range jobs {
				rec := CDRRecord{
					MSISDN:          fmt.Sprintf("+58414%07d", i%1000),
					BucketFecha:     bucketHoy,
					TimestampEvento: time.Now().UTC(),
					IDCDR:           fmt.Sprintf("cdr-%d-%d", workerID, i),
					TipoEvento:      "DATOS_5G",
					MontoCentavos:   150 + int64(i%300),
					Celda5G:         "CCS-NORTE-01",
				}

				t0 := time.Now()
				if err := escribirCDR(rec); err == nil {
					elapsedUs := float64(time.Since(t0).Microseconds())
					mu.Lock()
					latenciasUs = append(latenciasUs, elapsedUs)
					mu.Unlock()
				}
			}
		}(w)
	}

	for i := 0; i < totalOps; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	duracionTotal := time.Since(inicioTotal).Seconds()
	sort.Float64s(latenciasUs)

	throughput := float64(len(latenciasUs)) / duracionTotal
	p50 := percentil(latenciasUs, 50)
	p95 := percentil(latenciasUs, 95)
	p99 := percentil(latenciasUs, 99)

	fmt.Printf("Operaciones completadas: %d en %.3fs\n", len(latenciasUs), duracionTotal)
	fmt.Printf("Throughput: %.2f ops/s\n", throughput)
	fmt.Printf("Latencias (us) -> P50: %.2f us | P95: %.2f us | P99: %.2f us\n", p50, p95, p99)
}

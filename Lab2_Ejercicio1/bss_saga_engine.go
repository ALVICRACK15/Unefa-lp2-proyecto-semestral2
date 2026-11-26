package main

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrMontoInvalido       = errors.New("el monto en centavos debe ser mayor que cero")
	ErrMonedaIncompatible  = errors.New("las monedas no coinciden")
	ErrSaldoInsuficiente   = errors.New("saldo insuficiente para completar la operación")
	ErrSuscripcionInactiva = errors.New("la suscripción 5G no está activa")
)

type Dinero struct {
	MontoCentavos int64
	Moneda        string
}

func NuevoDinero(montoCentavos int64, moneda string) (Dinero, error) {
	if montoCentavos < 0 {
		return Dinero{}, ErrMontoInvalido
	}
	return Dinero{MontoCentavos: montoCentavos, Moneda: moneda}, nil
}

func (d Dinero) Sumar(otro Dinero) (Dinero, error) {
	if d.Moneda != otro.Moneda {
		return Dinero{}, ErrMonedaIncompatible
	}
	if otro.MontoCentavos <= 0 {
		return Dinero{}, ErrMontoInvalido
	}
	return Dinero{
		MontoCentavos: d.MontoCentavos + otro.MontoCentavos,
		Moneda:        d.Moneda,
	}, nil
}

func (d Dinero) Restar(otro Dinero) (Dinero, error) {
	if d.Moneda != otro.Moneda {
		return Dinero{}, ErrMonedaIncompatible
	}
	if otro.MontoCentavos <= 0 {
		return Dinero{}, ErrMontoInvalido
	}
	if d.MontoCentavos < otro.MontoCentavos {
		return Dinero{}, ErrSaldoInsuficiente
	}
	return Dinero{
		MontoCentavos: d.MontoCentavos - otro.MontoCentavos,
		Moneda:        d.Moneda,
	}, nil
}

type CargoAplicadoEvent struct {
	EventoID       string
	IdempotencyKey string
	SuscripcionID  string
	CargoCentavos  int64
	SaldoRestante  int64
	OcurridoEn     time.Time
}

type Suscripcion5G struct {
	ID         string
	MSISDN     string
	Activa     bool
	Saldo      Dinero
	Eventos    []CargoAplicadoEvent
	procesadas map[string]CargoAplicadoEvent
}

func NuevaSuscripcion5G(id, msisdn string, saldoInicial Dinero) *Suscripcion5G {
	return &Suscripcion5G{
		ID:         id,
		MSISDN:     msisdn,
		Activa:     true,
		Saldo:      saldoInicial,
		Eventos:    make([]CargoAplicadoEvent, 0),
		procesadas: make(map[string]CargoAplicadoEvent),
	}
}

func (s *Suscripcion5G) Debitar(idempotencyKey string, cargo Dinero) (CargoAplicadoEvent, error) {
	if prev, ok := s.procesadas[idempotencyKey]; ok {
		return prev, nil
	}
	if !s.Activa {
		return CargoAplicadoEvent{}, ErrSuscripcionInactiva
	}
	nuevoSaldo, err := s.Saldo.Restar(cargo)
	if err != nil {
		return CargoAplicadoEvent{}, err
	}
	s.Saldo = nuevoSaldo
	ev := CargoAplicadoEvent{
		EventoID:       fmt.Sprintf("evt-%d", len(s.Eventos)+1),
		IdempotencyKey: idempotencyKey,
		SuscripcionID:  s.ID,
		CargoCentavos:  cargo.MontoCentavos,
		SaldoRestante:  s.Saldo.MontoCentavos,
		OcurridoEn:     time.Now().UTC(),
	}
	s.Eventos = append(s.Eventos, ev)
	s.procesadas[idempotencyKey] = ev
	return ev, nil
}

func (s *Suscripcion5G) Acreditar(idempotencyKey string, abono Dinero) error {
	if !s.Activa {
		return ErrSuscripcionInactiva
	}
	nuevoSaldo, err := s.Saldo.Sumar(abono)
	if err != nil {
		return err
	}
	s.Saldo = nuevoSaldo
	_ = idempotencyKey
	return nil
}

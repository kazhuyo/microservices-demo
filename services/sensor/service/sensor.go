package service

import (
	"context"
	"database/sql"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/google/uuid"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"

	opentracingLog "github.com/opentracing/opentracing-go/log"
)

const (
	queryCreate = `INSERT INTO sensors (id, site_id, name, unit, min_safe, max_safe) VALUES ($1, $2, $3, $4, $5, $6)`
	queryAll    = `SELECT id, site_id, name, unit, min_safe, max_safe FROM sensors WHERE site_id = $1`
	queryGet    = `SELECT id, site_id, name, unit, min_safe, max_safe FROM sensors WHERE id = $1`
	queryUpdate = `UPDATE sensors SET site_id = $2, name = $3, unit = $4, min_safe = $5, max_safe = $6 WHERE id = $1`
	queryDelete = `DELETE FROM sensors WHERE id = $1`
)

type (
	// Sensor represents a Sensor in a site
	Sensor struct {
		ID      string  `json:"id"`
		SiteID  string  `json:"siteId"`
		Name    string  `json:"name"`
		Unit    string  `json:"unit"`
		MinSafe float64 `json:"minSafe"`
		MaxSafe float64 `json:"maxSafe"`
	}

	// SensorManager abstracts CRUD operations for Sensor
	SensorManager interface {
		Create(ctx context.Context, siteID, name, unit string, minSafe, maxSafe float64) (*Sensor, error)
		All(ctx context.Context, siteID string) ([]Sensor, error)
		Get(ctx context.Context, id string) (*Sensor, error)
		Update(ctx context.Context, s Sensor) (int, error)
		Delete(ctx context.Context, id string) error
	}

	postgresSensorManager struct {
		db     DB
		logger log.Logger
		tracer opentracing.Tracer
	}
)

// NewSensorManager creates a new sensor manager
func NewSensorManager(db DB, logger log.Logger, tracer opentracing.Tracer) SensorManager {
	return &postgresSensorManager{
		db:     db,
		logger: logger,
		tracer: tracer,
	}
}

func (m *postgresSensorManager) exec(ctx context.Context, name, query string, fn func() error) error {
	parentSpan := opentracing.SpanFromContext(ctx)
	span := m.tracer.StartSpan(name, opentracing.ChildOf(parentSpan.Context()))
	defer span.Finish()

	// https://github.com/opentracing/specification/blob/master/semantic_conventions.md
	ext.DBType.Set(span, "sql")
	ext.DBStatement.Set(span, query)

	err := fn()
	if err != nil {
		_ = level.Error(m.logger).Log("message", err.Error())
	}

	span.LogFields(opentracingLog.String("event", name))
	if err == nil {
		span.LogFields(opentracingLog.String("message", "successful!"))
	} else {
		span.LogFields(opentracingLog.String("message", err.Error()))
	}

	return err
}

func (m *postgresSensorManager) Create(ctx context.Context, siteID, name, unit string, minSafe, maxSafe float64) (*Sensor, error) {
	sensor := &Sensor{
		ID:      uuid.New().String(),
		SiteID:  siteID,
		Name:    name,
		Unit:    unit,
		MinSafe: minSafe,
		MaxSafe: maxSafe,
	}

	err := m.exec(ctx, "insert-record", queryCreate, func() error {
		_, err := m.db.ExecContext(ctx, queryCreate, sensor.ID, sensor.SiteID, sensor.Name, sensor.Unit, sensor.MinSafe, sensor.MaxSafe)
		return err
	})

	if err != nil {
		return nil, err
	}
	return sensor, nil
}

func (m *postgresSensorManager) All(ctx context.Context, siteID string) ([]Sensor, error) {
	sensors := make([]Sensor, 0)

	err := m.exec(ctx, "select-records", queryAll, func() error {
		rows, err := m.db.QueryContext(ctx, queryAll, siteID)
		if err != nil {
			return err
		}

		for rows.Next() {
			sensor := Sensor{}
			err := rows.Scan(&sensor.ID, &sensor.SiteID, &sensor.Name, &sensor.Unit, &sensor.MinSafe, &sensor.MaxSafe)
			if err == nil {
				sensors = append(sensors, sensor)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	return sensors, nil
}

func (m *postgresSensorManager) Get(ctx context.Context, id string) (*Sensor, error) {
	sensor := new(Sensor)

	err := m.exec(ctx, "select-record", queryGet, func() error {
		row := m.db.QueryRowContext(ctx, queryGet, id)
		err := row.Scan(&sensor.ID, &sensor.SiteID, &sensor.Name, &sensor.Unit, &sensor.MinSafe, &sensor.MaxSafe)
		if err == sql.ErrNoRows { // record does not exist
			sensor = nil
			return nil
		}
		return err
	})

	if err != nil {
		return nil, err
	}
	return sensor, nil
}

func (m *postgresSensorManager) Update(ctx context.Context, s Sensor) (int, error) {
	var n int64

	err := m.exec(ctx, "update-record", queryUpdate, func() error {
		res, err := m.db.ExecContext(ctx, queryUpdate, s.ID, s.SiteID, s.Name, s.Unit, s.MinSafe, s.MaxSafe)
		if err != nil {
			return err
		}

		n, err = res.RowsAffected()
		return err
	})

	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func (m *postgresSensorManager) Delete(ctx context.Context, id string) error {
	err := m.exec(ctx, "delete-record", queryDelete, func() error {
		_, err := m.db.ExecContext(ctx, queryDelete, id)
		return err
	})

	return err
}

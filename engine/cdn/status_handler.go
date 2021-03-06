package cdn

import (
	"context"
	"fmt"
	"net/http"

	"github.com/ovh/cds/engine/cdn/storage"
	"github.com/ovh/cds/engine/service"
	"github.com/ovh/cds/sdk"
)

func (s *Service) statusHandler() service.Handler {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		var status = http.StatusOK
		return service.WriteJSON(w, s.Status(ctx), status)
	}
}

func addMonitoringLine(nb int64, text string, err error, status string) sdk.MonitoringStatusLine {
	if err != nil {
		return sdk.MonitoringStatusLine{
			Component: text,
			Value:     fmt.Sprintf("Error: %v", err),
			Status:    sdk.MonitoringStatusAlert,
		}
	}
	return sdk.MonitoringStatusLine{
		Component: text,
		Value:     fmt.Sprintf("%d", nb),
		Status:    status,
	}
}

// Status returns the monitoring status for this service
func (s *Service) Status(ctx context.Context) *sdk.MonitoringStatus {
	m := s.NewMonitoringStatus()

	if !s.Cfg.EnableLogProcessing {
		return m
	}
	db := s.mustDBWithCtx(ctx)

	nbCompleted, err := storage.CountItemCompleted(db)
	m.AddLine(addMonitoringLine(nbCompleted, "items/completed", err, sdk.MonitoringStatusOK))

	nbIncoming, err := storage.CountItemIncoming(db)
	m.AddLine(addMonitoringLine(nbIncoming, "items/incoming", err, sdk.MonitoringStatusOK))

	m.AddLine(s.LogCache.Status(ctx)...)
	m.AddLine(s.getStatusSyncLogs()...)

	for _, st := range s.Units.Storages {
		m.AddLine(st.Status(ctx)...)
		size, err := storage.CountItemUnitByUnit(db, st.ID())
		if nbCompleted-size >= 100 {
			m.AddLine(addMonitoringLine(size, "backend/"+st.Name()+"/items", err, sdk.MonitoringStatusWarn))
		} else {
			m.AddLine(addMonitoringLine(size, "backend/"+st.Name()+"/items", err, sdk.MonitoringStatusOK))
		}
	}

	m.AddLine(s.DBConnectionFactory.Status(ctx))

	return m
}

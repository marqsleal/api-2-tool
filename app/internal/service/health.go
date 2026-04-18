package service

import "github.com/marqsleal/api-2-tool/internal/domain"

type HealthService struct{}

func NewHealthService() HealthService {
	return HealthService{}
}

func (s HealthService) Status() domain.HealthStatus {
	return domain.HealthStatus{Status: "ok"}
}

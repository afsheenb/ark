package ports

import (
	"github.com/ark-network/ark/server/internal/core/domain"
)

// HashrateRepository defines the interface for hashrate data operations
type HashrateRepository interface {
	// SaveHashrate saves a hashrate data point
	SaveHashrate(data *domain.HashrateData) error
	
	// GetHashrateByHeight gets hashrate data for a specific block height
	GetHashrateByHeight(height int32) (*domain.HashrateData, error)
	
	// GetHashrateRange gets hashrate data for a range of block heights
	GetHashrateRange(fromHeight, toHeight int32) ([]*domain.HashrateData, error)
	
	// GetLatestHashrate gets the latest hashrate data
	GetLatestHashrate() (*domain.HashrateData, error)
	
	// DeleteHashrate deletes hashrate data for a specific block height
	DeleteHashrate(height int32) error
}

package ports

import "github.com/ark-network/ark/server/internal/core/domain"
import "context"

type RepoManager interface {
	// Original repositories
	Events() domain.RoundEventRepository
	Rounds() domain.RoundRepository
	Vtxos() domain.VtxoRepository
	Notes() domain.NoteRepository
	Entities() domain.EntityRepository
	MarketHourRepo() domain.MarketHourRepo
	RegisterEventsHandler(func(*domain.Round))
	Close()
	
	// Hashrate derivatives repositories
	ContractRepository() domain.ContractRepository
	HashrateRepository() domain.HashrateRepository
	OrderRepository() domain.OrderRepository
	TradeRepository() domain.TradeRepository
	GetTransactionRepository() TransactionRepository
	ExecuteInTransaction(ctx context.Context, fn func(ctx context.Context) error) error
	
	// Access to the scheduler service
	SchedulerService() SchedulerService
}

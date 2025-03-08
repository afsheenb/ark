package ports

import (
	"github.com/ark-network/ark/server/internal/core/domain"
)

// TransactionRepository defines the interface for transaction-related operations
type TransactionRepository interface {
	// GetContractUTXO retrieves the UTXO associated with a contract
	GetContractUTXO(contractID string, txID string) (*UTXO, error)
	
	// BuildTransaction builds a new transaction based on the provided parameters
	BuildTransaction(params domain.TxParams) (*domain.Transaction, error)
	
	// SaveContractTransaction saves a contract transaction record
	SaveContractTransaction(tx *domain.TxRecord) error
	
	// GetTransactionByID retrieves a transaction by its ID
	GetTransactionByID(txID string) (*domain.Transaction, error)
	
	// GetTransactionsByContractID retrieves all transactions for a contract
	GetTransactionsByContractID(contractID string) ([]*domain.TxRecord, error)
	
	// UpdateTransactionStatus updates the status of a transaction
	UpdateTransactionStatus(txID string, status string, blockHeight int32) error
}

// UTXO represents an unspent transaction output
type UTXO struct {
	TxID        string  // Transaction ID
	OutputIndex uint32  // Output index in the transaction
	Amount      uint64  // Amount in satoshis
	Script      []byte  // Output script
}

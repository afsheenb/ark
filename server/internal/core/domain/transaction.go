package domain

import (
	"fmt"
	"time"
	"github.com/google/uuid"
)

// Transaction status constants
const (
	TxStatusCreated    = "created"
	TxStatusSigned     = "signed"
	TxStatusBroadcast  = "broadcast"
	TxStatusConfirmed  = "confirmed"
	TxStatusRejected   = "rejected"
)

// Transaction type constants
const (
	TxTypeSetup       = "setup"
	TxTypeActivation  = "activation"
	TxTypeSettlement  = "settlement"
	TxTypeRefund      = "refund"
)

// TxInput represents a transaction input
type TxInput struct {
	TxID        string  // Transaction ID of the UTXO
	OutputIndex uint32  // Output index in the transaction
	Amount      uint64  // Amount in satoshis
	ScriptPath  string  // Script execution path (for script conditions)
}

// TxOutput represents a transaction output
type TxOutput struct {
	PubKey      string  // Public key or address
	Amount      uint64  // Amount in satoshis
	Script      []byte  // Optional custom script
}

// TxParams contains parameters for building a transaction
type TxParams struct {
	Inputs       []TxInput   // Transaction inputs
	Outputs      []TxOutput  // Transaction outputs
	CurrentTime  int64       // Current Unix timestamp
	CurrentBlock int32       // Current blockchain height
	FeeRate      uint        // Fee rate in sat/vbyte
	Timelock     int64       // Optional timelock (0 means no timelock)
}

// Transaction represents a Bitcoin transaction
type Transaction struct {
	TxID        string      // Transaction ID
	RawTx       []byte      // Raw transaction bytes
	Inputs      []TxInput   // Transaction inputs
	Outputs     []TxOutput  // Transaction outputs
	Fee         uint64      // Transaction fee in satoshis
	Status      string      // Transaction status
	CreatedAt   time.Time   // Creation time
}

// NewTransaction creates a new transaction
func NewTransaction(params TxParams) (*Transaction, error) {
	if len(params.Inputs) == 0 {
		return nil, fmt.Errorf("transaction must have at least one input")
	}
	
	if len(params.Outputs) == 0 {
		return nil, fmt.Errorf("transaction must have at least one output")
	}
	
	// In a real implementation, this would build the actual transaction
	// For now, we'll just create a placeholder
	tx := &Transaction{
		Inputs:    params.Inputs,
		Outputs:   params.Outputs,
		Status:    TxStatusCreated,
		CreatedAt: time.Now(),
	}
	
	return tx, nil
}

// TxRecord represents a record of a contract-related transaction
type TxRecord struct {
	ID           string    // Unique identifier
	ContractID   string    // Associated contract ID
	TxID         string    // Transaction ID
	Type         string    // Transaction type
	Amount       uint64    // Amount in satoshis
	Timestamp    int64     // Unix timestamp
	BlockHeight  int32     // Block height (0 if not confirmed)
	Status       string    // Transaction status
	WinnerPubkey string    // Winner's public key (for settlement transactions)
	CreatedAt    time.Time // Creation time
	UpdatedAt    time.Time // Last update time
}

// NewTxRecord creates a new transaction record
func NewTxRecord(
	contractID string,
	txID string,
	txType string,
	amount uint64,
	timestamp int64,
	blockHeight int32,
	status string,
) *TxRecord {
	now := time.Now()
	return &TxRecord{
		ID:          uuid.New().String(),
		ContractID:  contractID,
		TxID:        txID,
		Type:        txType,
		Amount:      amount,
		Timestamp:   timestamp,
		BlockHeight: blockHeight,
		Status:      status,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// Update updates the transaction record status
func (r *TxRecord) Update(status string, blockHeight int32) {
	r.Status = status
	r.BlockHeight = blockHeight
	r.UpdatedAt = time.Now()
}

// SetWinner sets the winner's public key for settlement transactions
func (r *TxRecord) SetWinner(pubkey string) {
	r.WinnerPubkey = pubkey
	r.UpdatedAt = time.Now()
}

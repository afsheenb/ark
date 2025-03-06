package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// ContractType defines the type of hashrate contract
type ContractType string

const (
	// ContractTypeCall represents a CALL option (buyer wins if hashrate > strike)
	ContractTypeCall ContractType = "CALL"
	// ContractTypePut represents a PUT option (buyer wins if hashrate < strike)
	ContractTypePut ContractType = "PUT"
)

// ContractStatus defines the status of a hashrate contract
type ContractStatus string

const (
	// ContractStatusCreated indicates a contract has been created but not yet set up
	ContractStatusCreated ContractStatus = "CREATED"
	// ContractStatusSetup indicates a contract has been set up with initial transaction
	ContractStatusSetup ContractStatus = "SETUP"
	// ContractStatusActive indicates a contract is active and awaiting settlement
	ContractStatusActive ContractStatus = "ACTIVE"
	// ContractStatusSettled indicates a contract has been settled
	ContractStatusSettled ContractStatus = "SETTLED"
	// ContractStatusExpired indicates a contract has expired without being set up
	ContractStatusExpired ContractStatus = "EXPIRED"
	// ContractStatusFailed indicates a contract failed for some reason
	ContractStatusFailed ContractStatus = "FAILED"
)

// HashrateContract represents a Bitcoin hashrate derivative contract
type HashrateContract struct {
	ID               string         `json:"id"`
	ContractType     ContractType   `json:"contract_type"`
	StrikeHashRate   float64        `json:"strike_hash_rate"` // in EH/s
	StartBlockHeight int32          `json:"start_block_height"`
	EndBlockHeight   int32          `json:"end_block_height"`
	TargetTimestamp  int64          `json:"target_timestamp"`
	ContractSize     uint64         `json:"contract_size"` // in satoshis
	Premium          uint64         `json:"premium"`       // in satoshis
	BuyerPubkey      string         `json:"buyer_pubkey"`
	SellerPubkey     string         `json:"seller_pubkey"`
	Status           ContractStatus `json:"status"`
	CreatedAt        int64          `json:"created_at"`
	UpdatedAt        int64          `json:"updated_at"`
	ExpiresAt        int64          `json:"expires_at"`
	SetupTxid        string         `json:"setup_txid,omitempty"`
	FinalTxid        string         `json:"final_txid,omitempty"`
	SettlementTxid   string         `json:"settlement_txid,omitempty"`
}

// NewHashrateContract creates a new hashrate contract with default values
func NewHashrateContract(
	contractType ContractType,
	strikeHashRate float64,
	startBlockHeight int32,
	endBlockHeight int32,
	targetTimestamp int64,
	contractSize uint64,
	premium uint64,
	buyerPubkey string,
	sellerPubkey string,
	expiresAt int64,
) (*HashrateContract, error) {
	// Validate inputs
	if strikeHashRate <= 0 {
		return nil, errors.New("strike hash rate must be positive")
	}
	if startBlockHeight >= endBlockHeight {
		return nil, errors.New("end block height must be greater than start block height")
	}
	if targetTimestamp <= time.Now().Unix() {
		return nil, errors.New("target timestamp must be in the future")
	}
	if contractSize == 0 {
		return nil, errors.New("contract size must be positive")
	}
	if buyerPubkey == "" || sellerPubkey == "" {
		return nil, errors.New("buyer and seller pubkeys must be specified")
	}

	now := time.Now().Unix()
	if expiresAt == 0 {
		// Default expiry to 24 hours if not specified
		expiresAt = now + 86400
	}

	return &HashrateContract{
		ID:               uuid.New().String(),
		ContractType:     contractType,
		StrikeHashRate:   strikeHashRate,
		StartBlockHeight: startBlockHeight,
		EndBlockHeight:   endBlockHeight,
		TargetTimestamp:  targetTimestamp,
		ContractSize:     contractSize,
		Premium:          premium,
		BuyerPubkey:      buyerPubkey,
		SellerPubkey:     sellerPubkey,
		Status:           ContractStatusCreated,
		CreatedAt:        now,
		UpdatedAt:        now,
		ExpiresAt:        expiresAt,
	}, nil
}

// SetupContract transitions a contract to the SETUP status
func (c *HashrateContract) SetupContract(setupTxid string) error {
	if c.Status != ContractStatusCreated {
		return errors.New("contract must be in CREATED status to be set up")
	}
	if setupTxid == "" {
		return errors.New("setup txid must be specified")
	}

	c.Status = ContractStatusSetup
	c.SetupTxid = setupTxid
	c.UpdatedAt = time.Now().Unix()
	return nil
}

// ActivateContract transitions a contract to the ACTIVE status
func (c *HashrateContract) ActivateContract(finalTxid string) error {
	if c.Status != ContractStatusSetup {
		return errors.New("contract must be in SETUP status to be activated")
	}
	if finalTxid == "" {
		return errors.New("final txid must be specified")
	}

	c.Status = ContractStatusActive
	c.FinalTxid = finalTxid
	c.UpdatedAt = time.Now().Unix()
	return nil
}

// SettleContract transitions a contract to the SETTLED status
func (c *HashrateContract) SettleContract(settlementTxid string) error {
	if c.Status != ContractStatusActive {
		return errors.New("contract must be in ACTIVE status to be settled")
	}
	if settlementTxid == "" {
		return errors.New("settlement txid must be specified")
	}

	c.Status = ContractStatusSettled
	c.SettlementTxid = settlementTxid
	c.UpdatedAt = time.Now().Unix()
	return nil
}

// ExpireContract transitions a contract to the EXPIRED status
func (c *HashrateContract) ExpireContract() error {
	if c.Status == ContractStatusSettled {
		return errors.New("settled contracts cannot expire")
	}

	c.Status = ContractStatusExpired
	c.UpdatedAt = time.Now().Unix()
	return nil
}

// FailContract transitions a contract to the FAILED status
func (c *HashrateContract) FailContract() error {
	if c.Status == ContractStatusSettled {
		return errors.New("settled contracts cannot fail")
	}

	c.Status = ContractStatusFailed
	c.UpdatedAt = time.Now().Unix()
	return nil
}

// DetermineWinner determines the winner of the contract based on actual outcomes
// Returns the pubkey of the winner
func (c *HashrateContract) DetermineWinner(actualBlockHeight int32, actualTimestamp int64) (string, error) {
	if c.Status != ContractStatusActive {
		return "", errors.New("contract must be in ACTIVE status to determine winner")
	}

	// Enhanced calculation to handle edge cases
	// - If EndBlockHeight is reached before TargetTimestamp: High hashrate (blocks mined faster than expected)
	// - If TargetTimestamp is reached before EndBlockHeight: Low hashrate (blocks mined slower than expected)
	// - If both conditions are exactly met: Consider as low hashrate (conservative approach)
	// - If neither condition is met: Contract cannot be settled yet
	
	var highHashrateRealized bool
	
	if actualBlockHeight >= c.EndBlockHeight && actualTimestamp < c.TargetTimestamp {
		// Blocks were mined faster than expected timeframe
		highHashrateRealized = true
	} else if actualTimestamp >= c.TargetTimestamp {
		// Target time has passed, regardless of block height
		highHashrateRealized = false
	} else {
		// Neither condition is met, contract is still in progress
		return "", errors.New("contract conditions not yet met for settlement")
	}

	// CALL options pay out when hashrate is higher than expected (more blocks mined before timestamp)
	// PUT options pay out when hashrate is lower than expected (timestamp reached before blocks mined)
	switch c.ContractType {
	case ContractTypeCall:
		if highHashrateRealized {
			return c.BuyerPubkey, nil // Buyer wins on CALL if hashrate is high
		}
		return c.SellerPubkey, nil // Seller wins on CALL if hashrate is low
	case ContractTypePut:
		if highHashrateRealized {
			return c.SellerPubkey, nil // Seller wins on PUT if hashrate is high
		}
		return c.BuyerPubkey, nil // Buyer wins on PUT if hashrate is low
	default:
		return "", errors.New("unknown contract type")
	}
}

// CanSettle checks if the contract can be settled based on current blockchain state
func (c *HashrateContract) CanSettle(currentBlockHeight int32, currentTimestamp int64) (bool, string, error) {
	if c.Status != ContractStatusActive {
		return false, "", errors.New("contract must be in ACTIVE status to settle")
	}

	// Contract can be settled if either:
	// 1. We've reached the end block height (end condition for high hashrate)
	// 2. We've reached the target timestamp (end condition for low hashrate)
	
	// Calculate settleable conditions
	canSettle := false
	
	// If either condition is met, we can attempt settlement
	if currentBlockHeight >= c.EndBlockHeight || currentTimestamp >= c.TargetTimestamp {
		canSettle = true
	}
	
	if !canSettle {
		return false, "", nil
	}
	
	// If we can settle, determine the winner
	winner, err := c.DetermineWinner(currentBlockHeight, currentTimestamp)
	if err != nil {
		// Special case: if the error is because conditions aren't yet met, 
		// we return false instead of error
		if err.Error() == "contract conditions not yet met for settlement" {
			return false, "", nil
		}
		return false, "", err
	}
	
	return true, winner, nil
}

// IsExpired checks if the contract has expired
func (c *HashrateContract) IsExpired(currentTime int64) bool {
	return c.ExpiresAt > 0 && currentTime > c.ExpiresAt
}

// ContractRepository is the interface for hashrate contract storage
type ContractRepository interface {
	// Save stores a hashrate contract
	Save(contract *HashrateContract) error
	
	// FindByID retrieves a hashrate contract by ID
	FindByID(id string) (*HashrateContract, error)
	
	// FindByStatus retrieves hashrate contracts by status
	FindByStatus(status ContractStatus) ([]*HashrateContract, error)
	
	// FindByUser retrieves hashrate contracts where user is buyer or seller
	FindByUser(pubkey string) ([]*HashrateContract, error)
	
	// FindAll retrieves all hashrate contracts with optional pagination
	FindAll(limit, offset int) ([]*HashrateContract, error)
	
	// CountAll counts all hashrate contracts
	CountAll() (int, error)
}

// HashrateData represents a historical hashrate measurement
type HashrateData struct {
	BlockHeight int32   `json:"block_height"`
	Timestamp   int64   `json:"timestamp"`
	Hashrate    float64 `json:"hashrate"` // in EH/s
	Difficulty  uint64  `json:"difficulty"`
}

// HashrateRepository is the interface for hashrate data storage
type HashrateRepository interface {
	// SaveHashrate stores a hashrate measurement
	SaveHashrate(data *HashrateData) error
	
	// GetHashrateAt retrieves the hashrate at a specific block height
	GetHashrateAt(blockHeight int32) (*HashrateData, error)
	
	// GetHashrateRange retrieves hashrates between two block heights
	GetHashrateRange(startHeight, endHeight int32) ([]*HashrateData, error)
	
	// GetLatestHashrate retrieves the most recent hashrate measurement
	GetLatestHashrate() (*HashrateData, error)
}
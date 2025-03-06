package application

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/ark-network/ark/server/internal/core/domain"
	"github.com/ark-network/ark/server/internal/core/ports"
)

// ContractService manages Bitcoin hashrate derivative contracts
type ContractService struct {
	repos          ports.RepoManager
	hashrateCalc   *HashrateCalculator
	btcWallet      BtcWalletService
	scheduler      ports.SchedulerService
	eventPublisher EventPublisher
}

// NewContractService creates a new contract service
func NewContractService(
	repos ports.RepoManager,
	btcWallet BtcWalletService,
	scheduler ports.SchedulerService,
	eventPublisher EventPublisher,
) *ContractService {
	return &ContractService{
		repos:          repos,
		hashrateCalc:   NewHashrateCalculator(btcWallet),
		btcWallet:      btcWallet,
		scheduler:      scheduler,
		eventPublisher: eventPublisher,
	}
}

// CreateContract creates a new hashrate contract
func (s *ContractService) CreateContract(
	contractType domain.ContractType,
	strikeHashRate float64,
	startBlockHeight int32,
	endBlockHeight int32,
	targetTimestamp int64,
	contractSize uint64,
	premium uint64,
	buyerPubkey string,
	sellerPubkey string,
	expiresAt int64,
) (*domain.HashrateContract, error) {
	// Validate inputs via domain model constructor
	contract, err := domain.NewHashrateContract(
		contractType,
		strikeHashRate,
		startBlockHeight,
		endBlockHeight,
		targetTimestamp,
		contractSize,
		premium,
		buyerPubkey,
		sellerPubkey,
		expiresAt,
	)
	if err != nil {
		return nil, err
	}

	// Calculate and store the hashrate for historical reference
	_, err = s.btcWallet.GetCurrentHeight()
	if err != nil {
		return nil, fmt.Errorf("failed to get current height: %w", err)
	}

	// Save contract
	if err := s.repos.ContractRepository().Save(contract); err != nil {
		return nil, fmt.Errorf("failed to save contract: %w", err)
	}

	// Schedule contract expiration check
	if expiresAt > 0 {
		s.scheduler.ScheduleTaskOnce(expiresAt, func() {
			s.checkContractExpiration(contract.ID)
		})
	}

	// Publish event
	s.eventPublisher.PublishContractCreated(contract)

	return contract, nil
}

// GetContract gets a contract by ID
func (s *ContractService) GetContract(id string) (*domain.HashrateContract, error) {
	return s.repos.ContractRepository().FindByID(id)
}

// ListContracts lists contracts with optional filters
func (s *ContractService) ListContracts(
	status domain.ContractStatus,
	userPubkey string,
	limit, offset int,
) ([]*domain.HashrateContract, int, error) {
	var contracts []*domain.HashrateContract
	var err error
	var total int

	if userPubkey != "" {
		contracts, err = s.repos.ContractRepository().FindByUser(userPubkey)
		if err != nil {
			return nil, 0, err
		}

		// Filter by status if specified
		if status != "" {
			var filtered []*domain.HashrateContract
			for _, c := range contracts {
				if c.Status == status {
					filtered = append(filtered, c)
				}
			}
			contracts = filtered
		}

		total = len(contracts)

		// Apply pagination
		if limit > 0 {
			end := offset + limit
			if end > len(contracts) {
				end = len(contracts)
			}
			if offset < len(contracts) {
				contracts = contracts[offset:end]
			} else {
				contracts = []*domain.HashrateContract{}
			}
		}
	} else if status != "" {
		contracts, err = s.repos.ContractRepository().FindByStatus(status)
		if err != nil {
			return nil, 0, err
		}
		total = len(contracts)

		// Apply pagination
		if limit > 0 {
			end := offset + limit
			if end > len(contracts) {
				end = len(contracts)
			}
			if offset < len(contracts) {
				contracts = contracts[offset:end]
			} else {
				contracts = []*domain.HashrateContract{}
			}
		}
	} else {
		contracts, err = s.repos.ContractRepository().FindAll(limit, offset)
		if err != nil {
			return nil, 0, err
		}
		total, err = s.repos.ContractRepository().CountAll()
		if err != nil {
			return nil, 0, err
		}
	}

	return contracts, total, nil
}

// SetupContract sets up a contract with the initial transaction
func (s *ContractService) SetupContract(id, setupTxid string) (*domain.HashrateContract, error) {
	contract, err := s.repos.ContractRepository().FindByID(id)
	if err != nil {
		return nil, err
	}

	// Update contract state
	if err := contract.SetupContract(setupTxid); err != nil {
		return nil, err
	}

	// Save updated contract
	if err := s.repos.ContractRepository().Save(contract); err != nil {
		return nil, err
	}

	// Publish event
	s.eventPublisher.PublishContractSetup(id, setupTxid)

	return contract, nil
}

// ActivateContract activates a contract that has been set up
func (s *ContractService) ActivateContract(id, finalTxid string) (*domain.HashrateContract, error) {
	contract, err := s.repos.ContractRepository().FindByID(id)
	if err != nil {
		return nil, err
	}

	// Update contract state
	if err := contract.ActivateContract(finalTxid); err != nil {
		return nil, err
	}

	// Save updated contract
	if err := s.repos.ContractRepository().Save(contract); err != nil {
		return nil, err
	}

	// Schedule a settlement check when the contract end conditions may be reached
	s.scheduleSettlementCheck(contract)

	return contract, nil
}

// SettleContract settles a contract
func (s *ContractService) SettleContract(id, settlementTxid string) (*domain.HashrateContract, string, error) {
	contract, err := s.repos.ContractRepository().FindByID(id)
	if err != nil {
		return nil, "", err
	}

	// Get current blockchain state
	currentHeight, err := s.btcWallet.GetCurrentHeight()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get current height: %w", err)
	}

	currentTime := time.Now().Unix()

	// Check if contract can be settled
	canSettle, winnerPubkey, err := contract.CanSettle(currentHeight, currentTime)
	if err != nil {
		return nil, "", err
	}

	if !canSettle {
		return nil, "", errors.New("contract cannot be settled yet")
	}

	// Update contract state
	if err := contract.SettleContract(settlementTxid); err != nil {
		return nil, "", err
	}

	// Save updated contract
	if err := s.repos.ContractRepository().Save(contract); err != nil {
		return nil, "", err
	}

	// Publish event
	s.eventPublisher.PublishContractSettled(id, settlementTxid, winnerPubkey)

	return contract, winnerPubkey, nil
}

// CheckSettlement checks if a contract can be settled
func (s *ContractService) CheckSettlement(id string) (bool, string, error) {
	contract, err := s.repos.ContractRepository().FindByID(id)
	if err != nil {
		return false, "", err
	}

	// Get current blockchain state
	currentHeight, err := s.btcWallet.GetCurrentHeight()
	if err != nil {
		return false, "", fmt.Errorf("failed to get current height: %w", err)
	}

	currentTime := time.Now().Unix()

	// Check if contract can be settled
	return contract.CanSettle(currentHeight, currentTime)
}

// GetHashrate calculates the current Bitcoin network hashrate
func (s *ContractService) GetHashrate(windowSize int32) (float64, uint64, int32, int64, error) {
	if windowSize <= 0 {
		windowSize = 1008 // Default to approximately 1 week of blocks
	}

	// Get current height
	currentHeight, err := s.btcWallet.GetCurrentHeight()
	if err != nil {
		return 0, 0, 0, 0, err
	}

	// Calculate hashrate
	hashrate, difficulty, timestamp, err := s.hashrateCalc.CalculateHashrate(currentHeight, windowSize)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	return hashrate, difficulty, currentHeight, timestamp, nil
}

// GetHistoricalHashrates gets historical hashrate data
func (s *ContractService) GetHistoricalHashrates(
	fromHeight, toHeight int32,
	limit int,
) ([]*domain.HashrateData, error) {
	// Get current height if toHeight is not specified
	if toHeight <= 0 {
		var err error
		toHeight, err = s.btcWallet.GetCurrentHeight()
		if err != nil {
			return nil, err
		}
	}

	// Set default fromHeight if not specified
	if fromHeight <= 0 {
		fromHeight = toHeight - 1008 // Default to approximately 1 week of blocks
		if fromHeight < 1 {
			fromHeight = 1
		}
	}

	// Ensure fromHeight is less than toHeight
	if fromHeight >= toHeight {
		return nil, errors.New("fromHeight must be less than toHeight")
	}

	// Try to get data from repository first
	data, err := s.repos.HashrateRepository().GetHashrateRange(fromHeight, toHeight)
	if err == nil && len(data) > 0 {
		// Apply limit if specified
		if limit > 0 && len(data) > limit {
			data = data[len(data)-limit:]
		}
		return data, nil
	}

	// If no data in repository, calculate it
	// This is a simplified version that just calculates a few points
	// A real implementation would calculate more points and store them
	var results []*domain.HashrateData
	
	// Calculate number of points based on limit
	if limit <= 0 {
		limit = 10 // Default
	}

	step := (toHeight - fromHeight) / int32(limit)
	if step < 1 {
		step = 1
	}

	for height := fromHeight; height <= toHeight; height += step {
		// Calculate hashrate for this point
		hashrate, difficulty, timestamp, err := s.hashrateCalc.CalculateHashrate(height, 144) // Use 144 blocks (1 day) window
		if err != nil {
			continue
		}

		// Create and store data point
		dataPoint := &domain.HashrateData{
			BlockHeight: height,
			Timestamp:   timestamp,
			Hashrate:    hashrate,
			Difficulty:  difficulty,
		}

		// Save to repository for future use
		_ = s.repos.HashrateRepository().SaveHashrate(dataPoint)

		results = append(results, dataPoint)
	}

	return results, nil
}

// scheduleSettlementCheck schedules a check for contract settlement
func (s *ContractService) scheduleSettlementCheck(contract *domain.HashrateContract) {
	// Schedule based on target timestamp
	s.scheduler.ScheduleTaskOnce(contract.TargetTimestamp, func() {
		s.checkContractSettlement(contract.ID)
	})

	// Also schedule based on estimated time to reach end block height
	currentHeight, err := s.btcWallet.GetCurrentHeight()
	if err == nil && currentHeight < contract.EndBlockHeight {
		estimatedTime, err := s.hashrateCalc.CalculateTargetTimestamp(
			currentHeight,
			contract.EndBlockHeight,
			time.Now().Unix(),
		)
		if err == nil {
			s.scheduler.ScheduleTaskOnce(estimatedTime, func() {
				s.checkContractSettlement(contract.ID)
			})
		}
	}
}

// checkContractSettlement checks if a contract can be settled and settles it if possible
func (s *ContractService) checkContractSettlement(contractID string) {
	contract, err := s.repos.ContractRepository().FindByID(contractID)
	if err != nil {
		return
	}

	if contract.Status != domain.ContractStatusActive {
		return
	}

	// Check if can settle
	canSettle, winnerPubkey, err := s.CheckSettlement(contractID)
	if err != nil || !canSettle {
		return
	}

	// Build and execute the settlement transaction
	settlementTxid, err := s.buildAndExecuteSettlementTransaction(contract, winnerPubkey)
	if err != nil {
		// Log error but don't return, we'll retry later
		fmt.Printf("Failed to build settlement transaction: %v\n", err)
		return
	}
	
	if settlementTxid == "" {
		// No transaction was created, retry later
		s.scheduler.ScheduleTaskOnce(time.Now().Add(1*time.Hour).Unix(), func() {
			s.checkContractSettlement(contractID)
		})
		return
	}

	// Update contract
	if err := contract.SettleContract(settlementTxid); err != nil {
		return
	}

	// Save updated contract
	if err := s.repos.ContractRepository().Save(contract); err != nil {
		return
	}

	// Publish event
	s.eventPublisher.PublishContractSettled(contractID, settlementTxid, winnerPubkey)
}

// checkContractExpiration checks if a contract has expired and updates its status
func (s *ContractService) checkContractExpiration(contractID string) {
	contract, err := s.repos.ContractRepository().FindByID(contractID)
	if err != nil {
		return
	}

	currentTime := time.Now().Unix()
	if !contract.IsExpired(currentTime) {
		return
	}

	// Update contract status
	if err := contract.ExpireContract(); err != nil {
		return
	}

	// Save updated contract
	if err := s.repos.ContractRepository().Save(contract); err != nil {
		return
	}
}

// EventPublisher interface for publishing contract events
type EventPublisher interface {
	// PublishContractCreated publishes a contract created event
	PublishContractCreated(contract *domain.HashrateContract)
	
	// PublishContractSetup publishes a contract setup event
	PublishContractSetup(contractID, setupTxid string)
	
	// PublishContractSettled publishes a contract settled event
	PublishContractSettled(contractID, settlementTxid, winnerPubkey string)
}

// buildAndExecuteSettlementTransaction builds and broadcasts a settlement transaction
// for a hashrate contract. It returns the transaction ID or an error.
func (s *ContractService) buildAndExecuteSettlementTransaction(
	contract *domain.HashrateContract, 
	winnerPubkey string,
) (string, error) {
	if contract == nil {
		return "", errors.New("contract cannot be nil")
	}
	
	if contract.Status != domain.ContractStatusActive {
		return "", fmt.Errorf("contract must be active, current status: %s", contract.Status)
	}
	
	if winnerPubkey == "" {
		return "", errors.New("winner public key cannot be empty")
	}
	
	// 1. Get the contract utxo from the final transaction
	if contract.FinalTxID == "" {
		return "", errors.New("contract does not have a final transaction")
	}
	
	// Get contract utxo details (transaction output that represents the contract funds)
	contractUTXO, err := s.repos.TransactionRepository().GetContractUTXO(contract.ID, contract.FinalTxID)
	if err != nil {
		return "", fmt.Errorf("failed to get contract UTXO: %w", err)
	}
	
	// 2. Find the corresponding script path for the winner
	var scriptPathName string
	if contract.Type == domain.ContractTypeCall {
		// For CALL options, high hashrate means buyer wins
		if winnerPubkey == contract.BuyerPubkey {
			scriptPathName = "high_hashrate_path"
		} else {
			scriptPathName = "low_hashrate_path"
		}
	} else {
		// For PUT options, low hashrate means buyer wins
		if winnerPubkey == contract.BuyerPubkey {
			scriptPathName = "low_hashrate_path"
		} else {
			scriptPathName = "high_hashrate_path"
		}
	}
	
	// 3. Get the current blockchain height and time
	currentHeight, err := s.btcWallet.GetCurrentHeight()
	if err != nil {
		return "", fmt.Errorf("failed to get current height: %w", err)
	}
	currentTime := time.Now().Unix()
	
	// 4. Create settlement transaction inputs
	txInputs := []domain.TransactionInput{
		{
			TxID:        contractUTXO.TxID,
			OutputIndex: contractUTXO.OutputIndex,
			Amount:      contractUTXO.Amount,
			ScriptPath:  scriptPathName,
		},
	}
	
	// 5. Create settlement transaction outputs
	winnerAmount := contractUTXO.Amount - 1000 // Subtract fee
	txOutputs := []domain.TransactionOutput{
		{
			PubKey: winnerPubkey,
			Amount: winnerAmount,
		},
	}
	
	// 6. Build the settlement transaction
	txParams := domain.TransactionParams{
		Inputs:       txInputs,
		Outputs:      txOutputs,
		CurrentTime:  currentTime,
		CurrentBlock: currentHeight,
		FeeRate:      5, // sat/vbyte
	}
	
	settlementTx, err := s.repos.TransactionRepository().BuildTransaction(txParams)
	if err != nil {
		return "", fmt.Errorf("failed to build settlement transaction: %w", err)
	}
	
	// 7. Sign the transaction
	signedTx, err := s.btcWallet.SignTransaction(settlementTx)
	if err != nil {
		return "", fmt.Errorf("failed to sign settlement transaction: %w", err)
	}
	
	// 8. Broadcast the transaction
	txid, err := s.btcWallet.BroadcastTransaction(signedTx)
	if err != nil {
		return "", fmt.Errorf("failed to broadcast settlement transaction: %w", err)
	}
	
	// 9. Record the settlement transaction in the repository
	settlementTxRecord := &domain.ContractTransaction{
		ContractID:   contract.ID,
		TxID:         txid,
		Type:         domain.TransactionTypeSettlement,
		Amount:       winnerAmount,
		Timestamp:    currentTime,
		BlockHeight:  currentHeight,
		Status:       domain.TransactionStatusBroadcast,
		WinnerPubkey: winnerPubkey,
	}
	
	err = s.repos.TransactionRepository().SaveContractTransaction(settlementTxRecord)
	if err != nil {
		// Log but continue, as the transaction is already broadcast
		fmt.Printf("Failed to save settlement transaction record: %v\n", err)
	}
	
	return txid, nil
}
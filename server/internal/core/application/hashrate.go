package application

import (
	"errors"
	"math"
	"time"
)

// HashrateCalculator provides functions for calculating Bitcoin network hashrate
type HashrateCalculator struct {
	btcWallet BtcWalletService
}

// NewHashrateCalculator creates a new hashrate calculator with the given wallet service
func NewHashrateCalculator(wallet BtcWalletService) *HashrateCalculator {
	return &HashrateCalculator{
		btcWallet: wallet,
	}
}

// CalculateHashrate calculates the Bitcoin network hashrate over a window of blocks
// Returns hashrate in EH/s (exahash per second)
func (h *HashrateCalculator) CalculateHashrate(endHeight int32, windowSize int32) (float64, uint64, int64, error) {
	if windowSize <= 0 {
		return 0, 0, 0, errors.New("window size must be positive")
	}

	// Get end block info
	endBlock, err := h.btcWallet.GetBlockInfo(endHeight)
	if err != nil {
		return 0, 0, 0, err
	}

	// Get start block info
	startHeight := endHeight - windowSize + 1
	if startHeight <= 0 {
		startHeight = 1
	}
	startBlock, err := h.btcWallet.GetBlockInfo(startHeight)
	if err != nil {
		return 0, 0, 0, err
	}

	// Calculate time difference
	timeDiffSeconds := endBlock.Timestamp - startBlock.Timestamp
	if timeDiffSeconds <= 0 {
		return 0, 0, 0, errors.New("invalid time difference between blocks")
	}

	// Get current difficulty
	difficulty := endBlock.Difficulty

	// Calculate blocks per second
	blocksPerSecond := float64(windowSize-1) / float64(timeDiffSeconds)

	// Calculate hashrate using the formula: H = D * 2^32 / (t * 10^12)
	// where D is difficulty, t is average block time in seconds
	// Output is in EH/s (exahash per second, or 10^18 hashes per second)
	hashrate := (float64(difficulty) * math.Pow(2, 32) / 600) * blocksPerSecond / 1e6

	return hashrate, difficulty, endBlock.Timestamp, nil
}

// CalculateTargetTimestamp estimates when a target block height will be reached
// based on current hashrate and difficulty
func (h *HashrateCalculator) CalculateTargetTimestamp(
	currentHeight int32,
	targetHeight int32,
	currentTimestamp int64,
) (int64, error) {
	if targetHeight <= currentHeight {
		return 0, errors.New("target height must be greater than current height")
	}

	// Get current block info for latest difficulty
	_, err := h.btcWallet.GetBlockInfo(currentHeight)
	if err != nil {
		return 0, err
	}

	// Use default timestamp if current timestamp is not provided
	if currentTimestamp == 0 {
		currentTimestamp = time.Now().Unix()
	}

	// Calculate number of blocks to be mined
	blocksToMine := targetHeight - currentHeight

	// Assume average block time of 600 seconds (Bitcoin's target)
	// This is a simplified approach; a more accurate prediction would
	// account for difficulty adjustments
	estimatedTimeSeconds := int64(blocksToMine) * 600

	// Calculate target timestamp
	targetTimestamp := currentTimestamp + estimatedTimeSeconds

	return targetTimestamp, nil
}

// BtcWalletService is an interface for accessing Bitcoin blockchain data
// This allows us to mock the wallet service for testing
type BtcWalletService interface {
	// GetBlockInfo retrieves information about a specific block
	GetBlockInfo(height int32) (*BlockInfo, error)
	
	// GetCurrentHeight returns the current blockchain height
	GetCurrentHeight() (int32, error)
}

// BlockInfo contains relevant information about a Bitcoin block
type BlockInfo struct {
	Height     int32  `json:"height"`
	Hash       string `json:"hash"`
	Timestamp  int64  `json:"timestamp"`
	Difficulty uint64 `json:"difficulty"`
}
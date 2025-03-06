package application

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/ark-network/ark/common"
	"github.com/ark-network/ark/common/bitcointree"
	"github.com/ark-network/ark/common/note"
	"github.com/ark-network/ark/common/tree"
	"github.com/ark-network/ark/server/internal/core/domain"
	"github.com/ark-network/ark/server/internal/core/ports"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/waddrmgr"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/jmoiron/sqlx"
	"github.com/lightningnetwork/lnd/lnwallet/chainfee"
	log "github.com/sirupsen/logrus"
)

const marketHourDelta = 5 * time.Minute

// txMutexPool manages a pool of mutexes for transaction processing
type txMutexPool struct {
	sync.RWMutex
	mutexes  map[string]*sync.Mutex
	lastUsed map[string]time.Time
}

// newTxMutexPool creates a new transaction mutex pool
func newTxMutexPool() *txMutexPool {
	pool := &txMutexPool{
		mutexes:  make(map[string]*sync.Mutex),
		lastUsed: make(map[string]time.Time),
	}
	
	// Start a background goroutine for periodic cleanup
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		
		for range ticker.C {
			pool.cleanup()
		}
	}()
	
	return pool
}

// acquire gets or creates a mutex for the given key and locks it
func (p *txMutexPool) acquire(key string) {
	p.Lock()
	mtx, exists := p.mutexes[key]
	if !exists {
		mtx = &sync.Mutex{}
		p.mutexes[key] = mtx
	}
	// Update last used time when acquiring
	p.lastUsed[key] = time.Now()
	p.Unlock()
	mtx.Lock()
}

// release unlocks the mutex for the given key
func (p *txMutexPool) release(key string) {
	p.Lock() // Using full lock to safely update last used time
	mtx, exists := p.mutexes[key]
	if exists {
		// Update last used time when releasing
		p.lastUsed[key] = time.Now()
		p.Unlock()
		mtx.Unlock()
	} else {
		p.Unlock()
	}
}

// cleanup removes unused mutexes from the pool
// Should be called periodically by a background goroutine
func (p *txMutexPool) cleanup() {
	p.Lock()
	defer p.Unlock()
	
	// Implementation that tracks and removes unused mutexes
	// We'll use a map to track last usage time
	// This map will be initialized in the newTxMutexPool function
	
	// Get current time for comparison
	now := time.Now()
	
	// Set threshold for cleanup (30 minutes of inactivity)
	threshold := 30 * time.Minute
	
	// Check each mutex and remove if unused for threshold time
	for key, lastUsed := range p.lastUsed {
		if now.Sub(lastUsed) > threshold {
			// Remove the mutex and its tracking entry if it's not currently locked
			if mtx, exists := p.mutexes[key]; exists {
				// Try to acquire the mutex to check if it's not in use
				// If we can acquire it, it's safe to remove
				if mtx.TryLock() {
					mtx.Unlock() // Unlock immediately
					delete(p.mutexes, key)
					delete(p.lastUsed, key)
				}
			}
		}
	}
}

// Helper methods for transaction locks
func (s *covenantlessService) acquireTransactionLock(mutexKey string) {
	s.txMutexPool.acquire(mutexKey)
}

func (s *covenantlessService) releaseTransactionLock(mutexKey string) {
	s.txMutexPool.release(mutexKey)
}


type covenantlessService struct {
	network             common.Network
	pubkey              *secp256k1.PublicKey
	vtxoTreeExpiry      common.RelativeLocktime
	roundInterval       int64
	unilateralExitDelay common.RelativeLocktime
	boardingExitDelay   common.RelativeLocktime

	nostrDefaultRelays []string

	wallet      ports.WalletService
	repoManager ports.RepoManager
	builder     ports.TxBuilder
	scanner     ports.BlockchainScanner
	sweeper     *sweeper

	txRequests *txRequestsQueue
	forfeitTxs *forfeitTxsMap

	eventsCh            chan domain.RoundEvent
	transactionEventsCh chan TransactionEvent

	// cached data for the current round
	currentRoundLock        sync.Mutex
	currentRound            *domain.Round
	treeSigningSessions     map[string]*musigSigningSession
	// Map to track signing session last activity time
	signingSessionsLastUsed map[string]time.Time
	signingSessionsLock     sync.RWMutex

	// Server signing key derived from wallet
	serverSigningKey    *secp256k1.PrivateKey
	serverSigningPubKey *secp256k1.PublicKey
	
	// Transaction processing mutex pool to prevent race conditions
	txMutexPool *txMutexPool

	// allowZeroFees is a temporary flag letting to submit redeem txs with zero miner fees
	// this should be removed after we migrate to transactions version 3
	allowZeroFees bool

	numOfBoardingInputs    int
	numOfBoardingInputsMtx sync.RWMutex

	forfeitsBoardingSigsChan chan struct{}
}

func NewCovenantlessService(
	network common.Network,
	roundInterval int64,
	vtxoTreeExpiry, unilateralExitDelay, boardingExitDelay common.RelativeLocktime,
	nostrDefaultRelays []string,
	walletSvc ports.WalletService, repoManager ports.RepoManager,
	builder ports.TxBuilder, scanner ports.BlockchainScanner,
	scheduler ports.SchedulerService,
	noteUriPrefix string,
	marketHourStartTime, marketHourEndTime time.Time,
	marketHourPeriod, marketHourRoundInterval time.Duration,
	allowZeroFees bool,
) (Service, error) {
	pubkey, err := walletSvc.GetPubkey(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pubkey: %s", err)
	}

	// Try to load market hours from DB first
	marketHour, err := repoManager.MarketHourRepo().Get(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get market hours from db: %w", err)
	}

	if marketHour == nil {
		marketHour = domain.NewMarketHour(marketHourStartTime, marketHourEndTime, marketHourPeriod, marketHourRoundInterval)
		if err := repoManager.MarketHourRepo().Upsert(context.Background(), *marketHour); err != nil {
			return nil, fmt.Errorf("failed to upsert initial market hours to db: %w", err)
		}
	}

	// Instead of generating a random key, derive the server signing key from the wallet
	// This ensures consistency across restarts
	serverSigningKey, err := walletSvc.DeriveSigningKey(context.Background(), "hashhedge/server_signing_key")
	if err != nil {
		return nil, fmt.Errorf("failed to derive server signing key from wallet: %s", err)
	}

	svc := &covenantlessService{
		network:                  network,
		pubkey:                   pubkey,
		vtxoTreeExpiry:           vtxoTreeExpiry,
		roundInterval:            roundInterval,
		unilateralExitDelay:      unilateralExitDelay,
		wallet:                   walletSvc,
		repoManager:              repoManager,
		builder:                  builder,
		scanner:                  scanner,
		sweeper:                  newSweeper(walletSvc, repoManager, builder, scheduler, noteUriPrefix),
		txRequests:               newTxRequestsQueue(),
		forfeitTxs:               newForfeitTxsMap(builder),
		eventsCh:                 make(chan domain.RoundEvent),
		transactionEventsCh:      make(chan TransactionEvent),
		currentRoundLock:         sync.Mutex{},
		treeSigningSessions:      make(map[string]*musigSigningSession),
		signingSessionsLastUsed:  make(map[string]time.Time),
		signingSessionsLock:      sync.RWMutex{},
		boardingExitDelay:        boardingExitDelay,
		nostrDefaultRelays:       nostrDefaultRelays,
		serverSigningKey:         serverSigningKey,
		serverSigningPubKey:      serverSigningKey.PubKey(),
		txMutexPool:              newTxMutexPool(),
		allowZeroFees:            allowZeroFees,
		forfeitsBoardingSigsChan: make(chan struct{}, 1),
	}

	repoManager.RegisterEventsHandler(
		func(round *domain.Round) {
			go func() {
				defer func() {
					if r := recover(); r != nil {
						log.Errorf("recovered from panic in propagateEvents: %v", r)
					}
				}()

				svc.propagateEvents(round)
			}()

			go func() {
				defer func() {
					if r := recover(); r != nil {
						log.Errorf("recovered from panic in updateVtxoSet and scheduleSweepVtxosForRound: %v", r)
					}
				}()

				// utxo db must be updated before scheduling the sweep events
				svc.updateVtxoSet(round)
				svc.scheduleSweepVtxosForRound(round)
			}()
		},
	)

	if err := svc.restoreWatchingVtxos(); err != nil {
		return nil, fmt.Errorf("failed to restore watching vtxos: %s", err)
	}
	go svc.listenToScannerNotifications()
	return svc, nil
}

// cleanupSigningSessions periodically removes old signing sessions to prevent memory leaks
func (s *covenantlessService) cleanupSigningSessions() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		func() {
			// Use defer to recover from panics to ensure the cleanup continues
			defer func() {
				if r := recover(); r != nil {
					log.Errorf("recovered from panic in cleanupSigningSessions: %v", r)
				}
			}()
			
			s.currentRoundLock.Lock()
			defer s.currentRoundLock.Unlock()
			
			// Get current time for comparison
			now := time.Now()
			
			// Set threshold for cleanup (1 hour of inactivity)
			threshold := 1 * time.Hour
			
			// Safely read the last used times
			s.signingSessionsLock.RLock()
			sessionsToRemove := []string{}
			for id, lastUsed := range s.signingSessionsLastUsed {
				if now.Sub(lastUsed) > threshold {
					sessionsToRemove = append(sessionsToRemove, id)
				}
			}
			s.signingSessionsLock.RUnlock()
			
			// Remove old sessions if any found
			if len(sessionsToRemove) > 0 {
				s.signingSessionsLock.Lock()
				for _, id := range sessionsToRemove {
					// Double-check time since we released the lock
					if lastUsed, exists := s.signingSessionsLastUsed[id]; exists && now.Sub(lastUsed) > threshold {
						delete(s.treeSigningSessions, id)
						delete(s.signingSessionsLastUsed, id)
						log.Infof("Cleaned up signing session %s due to inactivity", id)
					}
				}
				s.signingSessionsLock.Unlock()
			}
		}()
	}
}

func (s *covenantlessService) Start() error {
	log.Debug("starting sweeper service")
	if err := s.sweeper.start(); err != nil {
		return err
	}

	log.Debug("starting app service")
	go s.start()
	
	// Start the signing sessions cleanup goroutine
	go s.cleanupSigningSessions()
	
	return nil
}

func (s *covenantlessService) Stop() {
	s.sweeper.stop()
	// nolint
	vtxos, _ := s.repoManager.Vtxos().GetAllSweepableVtxos(context.Background())
	if len(vtxos) > 0 {
		s.stopWatchingVtxos(vtxos)
	}

	s.wallet.Close()
	log.Debug("closed connection to wallet")
	s.repoManager.Close()
	log.Debug("closed connection to db")
	close(s.eventsCh)
}

func (s *covenantlessService) SubmitRedeemTx(
	ctx context.Context, redeemTx string,
) (string, string, error) {
	if redeemTx == "" {
		return "", "", fmt.Errorf("empty redeem transaction")
	}
	
	// Parse the PSBT for initial validation
	redeemPtx, err := psbt.NewFromRawBytes(strings.NewReader(redeemTx), true)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse redeem tx: %w", err)
	}

	// Verify transaction structure has both inputs and outputs
	if len(redeemPtx.UnsignedTx.TxIn) == 0 {
		return "", "", fmt.Errorf("redeem transaction has no inputs")
	}
	
	if len(redeemPtx.UnsignedTx.TxOut) == 0 {
		return "", "", fmt.Errorf("redeem transaction has no outputs")
	}

	// Use a mutex pool based on VTXO to prevent race conditions
	// This ensures we don't try to spend the same VTXO in parallel requests
	// Generate a mutex key from the transaction inputs
	mutexKey := redeemPtx.UnsignedTx.TxIn[0].PreviousOutPoint.Hash.String()
	s.acquireTransactionLock(mutexKey)
	defer s.releaseTransactionLock(mutexKey)

	vtxoRepo := s.repoManager.Vtxos()

	expiration := int64(0)
	roundTxid := ""

	ins := make([]common.VtxoInput, 0)

	ptx, err := psbt.NewFromRawBytes(strings.NewReader(redeemTx), true)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse redeem tx: %w", err)
	}

	// Track VTXOs being spent
	spentVtxoKeys := make([]domain.VtxoKey, 0, len(ptx.Inputs))
	for _, input := range ptx.UnsignedTx.TxIn {
		spentVtxoKeys = append(spentVtxoKeys, domain.VtxoKey{
			Txid: input.PreviousOutPoint.Hash.String(),
			VOut: input.PreviousOutPoint.Index,
		})
	}

	// Fetch all VTXOs in a single query with a transaction
	var spentVtxos []domain.Vtxo
	err = s.repoManager.ExecuteInTransaction(ctx, func(tx *sqlx.Tx) error {
		var err error
		spentVtxos, err = vtxoRepo.GetVtxosForUpdate(ctx, spentVtxoKeys)
		if err != nil {
			return fmt.Errorf("failed to get vtxos with lock: %w", err)
		}
		
		if len(spentVtxos) != len(spentVtxoKeys) {
			return fmt.Errorf("some vtxos not found: expected %d, got %d", 
				len(spentVtxoKeys), len(spentVtxos))
		}
		
		// Validate none are already spent
		for _, vtxo := range spentVtxos {
			if vtxo.Spent {
				return fmt.Errorf("vtxo %s:%d already spent", vtxo.Txid, vtxo.VOut)
			}
			if vtxo.Redeemed {
				return fmt.Errorf("vtxo %s:%d already redeemed", vtxo.Txid, vtxo.VOut)
			}
			if vtxo.Swept {
				return fmt.Errorf("vtxo %s:%d already swept", vtxo.Txid, vtxo.VOut)
			}
		}
		
		return nil
	})
	
	if err != nil {
		return "", "", err
	}

	vtxoMap := make(map[wire.OutPoint]domain.Vtxo)
	for _, vtxo := range spentVtxos {
		hash, err := chainhash.NewHashFromStr(vtxo.Txid)
		if err != nil {
			return "", "", fmt.Errorf("failed to parse vtxo txid: %w", err)
		}
		vtxoMap[wire.OutPoint{Hash: *hash, Index: vtxo.VOut}] = vtxo
	}

	sumOfInputs := int64(0)
	for inputIndex, input := range ptx.Inputs {
		// Validate required PSBT fields exist
		if input.WitnessUtxo == nil {
			return "", "", fmt.Errorf("missing witness utxo for input %d", inputIndex)
		}

		if len(input.TaprootLeafScript) == 0 {
			return "", "", fmt.Errorf("missing tapscript leaf for input %d", inputIndex)
		}

		tapscript := input.TaprootLeafScript[0]

		if len(input.TaprootScriptSpendSig) == 0 {
			return "", "", fmt.Errorf("missing tapscript spend sig for input %d", inputIndex)
		}

		outpoint := ptx.UnsignedTx.TxIn[inputIndex].PreviousOutPoint

		vtxo, exists := vtxoMap[outpoint]
		if !exists {
			return "", "", fmt.Errorf("vtxo for input %d (%s:%d) not found in database", 
				inputIndex, outpoint.Hash.String(), outpoint.Index)
		}

		// Make sure we don't use the same vtxo twice within this transaction
		delete(vtxoMap, outpoint)

		sumOfInputs += input.WitnessUtxo.Value

		// Track expiration and round ID
		if inputIndex == 0 || vtxo.ExpireAt < expiration {
			roundTxid = vtxo.RoundTxid
			expiration = vtxo.ExpireAt
		}

		// Verify that the user signs a forfeit closure
		var userPubkey *secp256k1.PublicKey
		serverXOnlyPubkey := schnorr.SerializePubKey(s.pubkey)

		for _, sig := range input.TaprootScriptSpendSig {
			if !bytes.Equal(sig.XOnlyPubKey, serverXOnlyPubkey) {
				parsed, err := schnorr.ParsePubKey(sig.XOnlyPubKey)
				if err != nil {
					return "", "", fmt.Errorf("failed to parse pubkey for input %d: %w", 
						inputIndex, err)
				}
				userPubkey = parsed
				break
			}
		}

		if userPubkey == nil {
			return "", "", fmt.Errorf("input %d is not signed by the user", inputIndex)
		}

		// Verify the VTXO public key matches
		vtxoPubkeyBuf, err := hex.DecodeString(vtxo.PubKey)
		if err != nil {
			return "", "", fmt.Errorf("failed to decode vtxo pubkey for %s:%d: %w", 
				vtxo.Txid, vtxo.VOut, err)
		}

		vtxoPubkey, err := schnorr.ParsePubKey(vtxoPubkeyBuf)
		if err != nil {
			return "", "", fmt.Errorf("failed to parse vtxo pubkey for %s:%d: %w", 
				vtxo.Txid, vtxo.VOut, err)
		}

		// Verify witness utxo matches VTXO
		pkscript, err := common.P2TRScript(vtxoPubkey)
		if err != nil {
			return "", "", fmt.Errorf("failed to get pkscript for input %d: %w", inputIndex, err)
		}

		if !bytes.Equal(input.WitnessUtxo.PkScript, pkscript) {
			return "", "", fmt.Errorf("witness utxo script mismatch for input %d", inputIndex)
		}

		if input.WitnessUtxo.Value != int64(vtxo.Amount) {
			return "", "", fmt.Errorf("witness utxo value mismatch for input %d: expected %d, got %d", 
				inputIndex, vtxo.Amount, input.WitnessUtxo.Value)
		}

		// Verify forfeit closure script
		closure, err := tree.DecodeClosure(tapscript.Script)
		if err != nil {
			return "", "", fmt.Errorf("failed to decode forfeit closure for input %d: %w", 
				inputIndex, err)
		}

		var locktime *common.AbsoluteLocktime

		switch c := closure.(type) {
		case *tree.CLTVMultisigClosure:
			locktime = &c.Locktime
		case *tree.MultisigClosure, *tree.ConditionMultisigClosure:
			// These types don't have a locktime
		default:
			return "", "", fmt.Errorf("invalid forfeit closure script for input %d", inputIndex)
		}

		// Verify timelock if present
		if locktime != nil {
			blocktimestamp, err := s.wallet.GetCurrentBlockTime(ctx)
			if err != nil {
				return "", "", fmt.Errorf("failed to get current block time: %w", err)
			}
			if !locktime.IsSeconds() {
				if *locktime > common.AbsoluteLocktime(blocktimestamp.Height) {
					return "", "", fmt.Errorf("forfeit closure is CLTV locked for input %d, %d > %d (height)", 
						inputIndex, *locktime, blocktimestamp.Height)
				}
			} else {
				if *locktime > common.AbsoluteLocktime(blocktimestamp.Time) {
					return "", "", fmt.Errorf("forfeit closure is CLTV locked for input %d, %d > %d (time)", 
						inputIndex, *locktime, blocktimestamp.Time)
				}
			}
		}

		// Parse the control block
		ctrlBlock, err := txscript.ParseControlBlock(tapscript.ControlBlock)
		if err != nil {
			return "", "", fmt.Errorf("failed to parse control block for input %d: %w", 
				inputIndex, err)
		}

		// Add to inputs collection
		ins = append(ins, common.VtxoInput{
			Outpoint: &outpoint,
			Tapscript: &waddrmgr.Tapscript{
				ControlBlock:   ctrlBlock,
				RevealedScript: tapscript.Script,
			},
		})
	}

	// Verify outputs meet dust threshold
	dust, err := s.wallet.GetDustAmount(ctx)
	if err != nil {
		return "", "", fmt.Errorf("failed to get dust threshold: %w", err)
	}

	outputs := ptx.UnsignedTx.TxOut

	sumOfOutputs := int64(0)
	for outIndex, out := range outputs {
		sumOfOutputs += out.Value
		if out.Value < int64(dust) {
			return "", "", fmt.Errorf("output %d value (%d) is less than dust threshold (%d)", 
				outIndex, out.Value, dust)
		}
	}

	// Verify fees
	fees := sumOfInputs - sumOfOutputs
	if fees < 0 {
		return "", "", fmt.Errorf("invalid fees, inputs (%d) are less than outputs (%d)", 
			sumOfInputs, sumOfOutputs)
	}

	// Check minimum fee rate
	// NOTE: We always check minimum fee rate even if allowZeroFees is true
	// for security reasons, but will use a much lower threshold in that case
	
	// Use mutex to protect access to wallet fee rate APIs
	minFeeRate := s.wallet.MinRelayFeeRate(ctx)
	
	// If allowZeroFees is enabled, we'll still set a lower floor for security
	// but allow it to be much lower than the default minimum relay fee
	if s.allowZeroFees && fees > 0 {
		// If there is any fee at all and allowZeroFees is true, we'll accept it
		// as long as it's positive and won't make the tx too large
		log.Warnf("Allowing low fee transaction due to allowZeroFees=true: %d sats", fees)
	} else {
		// Normal fee validation logic
		minFees, err := common.ComputeRedeemTxFee(chainfee.SatPerKVByte(minFeeRate), ins, len(outputs))
		if err != nil {
			return "", "", fmt.Errorf("failed to compute min fees: %w", err)
		}
		
		if fees < minFees {
			return "", "", fmt.Errorf("min relay fee not met, %d < %d", fees, minFees)
		}
	}

	// Rebuild and verify redeem tx for security
	rebuiltRedeemTx, err := bitcointree.BuildRedeemTx(ins, outputs)
	if err != nil {
		return "", "", fmt.Errorf("failed to rebuild redeem tx: %w", err)
	}

	rebuiltPtx, err := psbt.NewFromRawBytes(strings.NewReader(rebuiltRedeemTx), true)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse rebuilt redeem tx: %w", err)
	}

	// Ensure the rebuilt transaction matches the submitted one
	rebuiltTxid := rebuiltPtx.UnsignedTx.TxID()
	redeemTxid := redeemPtx.UnsignedTx.TxID()
	if rebuiltTxid != redeemTxid {
		return "", "", fmt.Errorf("invalid redeem tx, computed txid (%s) doesn't match submitted txid (%s)",
			rebuiltTxid, redeemTxid)
	}

	// Verify transaction signatures
	if valid, _, err := s.builder.VerifyTapscriptPartialSigs(redeemTx); err != nil || !valid {
		return "", "", fmt.Errorf("invalid transaction signature: %w", err)
	}

	// Sign the transaction
	signedRedeemTx, err := s.wallet.SignTransactionTapscript(ctx, redeemTx, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to sign redeem tx: %w", err)
	}

	// Create new vtxos from outputs and mark inputs as spent
	// Wrap in a database transaction to ensure atomicity
	var newVtxos []domain.Vtxo
	
	err = s.repoManager.ExecuteInTransaction(ctx, func(tx *sqlx.Tx) error {
		// Create new VTXOs from outputs
		newVtxos = make([]domain.Vtxo, 0, len(redeemPtx.UnsignedTx.TxOut))
		for outIndex, out := range outputs {
			// Parse the output public key
			vtxoTapKey, err := schnorr.ParsePubKey(out.PkScript[2:])
			if err != nil {
				return fmt.Errorf("failed to parse vtxo taproot key for output %d: %w", 
					outIndex, err)
			}

			vtxoPubkey := hex.EncodeToString(schnorr.SerializePubKey(vtxoTapKey))

			// Create new VTXO
			newVtxos = append(newVtxos, domain.Vtxo{
				VtxoKey: domain.VtxoKey{
					Txid: redeemTxid,
					VOut: uint32(outIndex),
				},
				PubKey:    vtxoPubkey,
				Amount:    uint64(out.Value),
				ExpireAt:  expiration,
				RoundTxid: roundTxid,
				RedeemTx:  signedRedeemTx,
				CreatedAt: time.Now().Unix(),
			})
		}

		// Add new VTXOs
		if err := s.repoManager.Vtxos().AddVtxos(ctx, newVtxos); err != nil {
			return fmt.Errorf("failed to add vtxos: %w", err)
		}
		
		// Mark spent VTXOs
		if err := s.repoManager.Vtxos().SpendVtxos(ctx, spentVtxoKeys, redeemTxid); err != nil {
			return fmt.Errorf("failed to mark vtxos as spent: %w", err)
		}
		
		return nil
	})
	
	if err != nil {
		return "", "", fmt.Errorf("database transaction failed: %w", err)
	}
	
	log.Infof("Redeem transaction processed - spent %d vtxos, created %d new vtxos, txid: %s", 
		len(spentVtxos), len(newVtxos), redeemTxid)
		
	// Start watching new VTXOs (non-blocking)
	go func() {
		if err := s.startWatchingVtxos(newVtxos); err != nil {
			log.WithError(err).Warn("failed to start watching new vtxos")
		} else {
			log.Debugf("started watching %d new vtxos", len(newVtxos))
		}
	}()

	// Notify about transaction (non-blocking)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("recovered from panic in redeem transaction notification: %v", r)
				debug.PrintStack()
			}
		}()
		
		s.transactionEventsCh <- RedeemTransactionEvent{
			RedeemTxid:     redeemTxid,
			SpentVtxos:     spentVtxos,
			SpendableVtxos: newVtxos,
		}
	}()

	return signedRedeemTx, redeemTxid, nil
}

func (s *covenantlessService) GetBoardingAddress(
	ctx context.Context, userPubkey *secp256k1.PublicKey,
) (address string, scripts []string, err error) {
	vtxoScript := bitcointree.NewDefaultVtxoScript(s.pubkey, userPubkey, s.boardingExitDelay)

	tapKey, _, err := vtxoScript.TapTree()
	if err != nil {
		return "", nil, fmt.Errorf("failed to get taproot key: %s", err)
	}

	addr, err := btcutil.NewAddressTaproot(
		schnorr.SerializePubKey(tapKey), s.chainParams(),
	)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get address: %s", err)
	}

	scripts, err = vtxoScript.Encode()
	if err != nil {
		return "", nil, fmt.Errorf("failed to encode vtxo script: %s", err)
	}

	address = addr.EncodeAddress()

	return
}

func (s *covenantlessService) SpendNotes(ctx context.Context, notes []note.Note) (string, error) {
	notesRepo := s.repoManager.Notes()

	for _, note := range notes {
		// verify the note signature
		hash := note.Hash()

		valid, err := s.wallet.VerifyMessageSignature(ctx, hash, note.Signature)
		if err != nil {
			return "", fmt.Errorf("failed to verify note signature: %s", err)
		}

		if !valid {
			return "", fmt.Errorf("invalid note signature %s", note)
		}

		// verify that the note is spendable
		spent, err := notesRepo.Contains(ctx, note.ID)
		if err != nil {
			return "", fmt.Errorf("failed to check if note is spent: %s", err)
		}

		if spent {
			return "", fmt.Errorf("note already spent: %s", note)
		}
	}

	request, err := domain.NewTxRequest(make([]domain.Vtxo, 0))
	if err != nil {
		return "", fmt.Errorf("failed to create tx request: %s", err)
	}

	if err := s.txRequests.pushWithNotes(*request, notes); err != nil {
		return "", fmt.Errorf("failed to push tx requests: %s", err)
	}

	return request.Id, nil
}

func (s *covenantlessService) SpendVtxos(ctx context.Context, inputs []ports.Input) (string, error) {
	vtxosInputs := make([]domain.Vtxo, 0)
	boardingInputs := make([]ports.BoardingInput, 0)

	now := time.Now().Unix()

	boardingTxs := make(map[string]wire.MsgTx, 0) // txid -> txhex

	for _, input := range inputs {
		vtxosResult, err := s.repoManager.Vtxos().GetVtxos(ctx, []domain.VtxoKey{input.VtxoKey})
		if err != nil || len(vtxosResult) == 0 {
			// vtxo not found in db, check if it exists on-chain
			if _, ok := boardingTxs[input.Txid]; !ok {
				// check if the tx exists and is confirmed
				txhex, err := s.wallet.GetTransaction(ctx, input.Txid)
				if err != nil {
					return "", fmt.Errorf("failed to get tx %s: %s", input.Txid, err)
				}

				var tx wire.MsgTx
				if err := tx.Deserialize(hex.NewDecoder(strings.NewReader(txhex))); err != nil {
					return "", fmt.Errorf("failed to deserialize tx %s: %s", input.Txid, err)
				}

				confirmed, _, blocktime, err := s.wallet.IsTransactionConfirmed(ctx, input.Txid)
				if err != nil {
					return "", fmt.Errorf("failed to check tx %s: %s", input.Txid, err)
				}

				if !confirmed {
					return "", fmt.Errorf("tx %s not confirmed", input.Txid)
				}

				vtxoScript, err := bitcointree.ParseVtxoScript(input.Tapscripts)
				if err != nil {
					return "", fmt.Errorf("failed to parse boarding descriptor: %s", err)
				}

				exitDelay, err := vtxoScript.SmallestExitDelay()
				if err != nil {
					return "", fmt.Errorf("failed to get exit delay: %s", err)
				}

				// if the exit path is available, forbid registering the boarding utxo
				if blocktime+exitDelay.Seconds() < now {
					return "", fmt.Errorf("tx %s expired", input.Txid)
				}

				boardingTxs[input.Txid] = tx
			}

			tx := boardingTxs[input.Txid]
			boardingInput, err := s.newBoardingInput(tx, input)
			if err != nil {
				return "", err
			}

			boardingInputs = append(boardingInputs, *boardingInput)
			continue
		}

		vtxo := vtxosResult[0]
		if vtxo.Spent {
			return "", fmt.Errorf("input %s:%d already spent", vtxo.Txid, vtxo.VOut)
		}

		if vtxo.Redeemed {
			return "", fmt.Errorf("input %s:%d already redeemed", vtxo.Txid, vtxo.VOut)
		}

		if vtxo.Swept {
			return "", fmt.Errorf("input %s:%d already swept", vtxo.Txid, vtxo.VOut)
		}

		vtxoScript, err := bitcointree.ParseVtxoScript(input.Tapscripts)
		if err != nil {
			return "", fmt.Errorf("failed to parse boarding descriptor: %s", err)
		}

		tapKey, _, err := vtxoScript.TapTree()
		if err != nil {
			return "", fmt.Errorf("failed to get taproot key: %s", err)
		}

		expectedTapKey, err := vtxo.TapKey()
		if err != nil {
			return "", fmt.Errorf("failed to get taproot key: %s", err)
		}

		if !bytes.Equal(schnorr.SerializePubKey(tapKey), schnorr.SerializePubKey(expectedTapKey)) {
			return "", fmt.Errorf("descriptor does not match vtxo pubkey")
		}

		vtxosInputs = append(vtxosInputs, vtxo)
	}

	request, err := domain.NewTxRequest(vtxosInputs)
	if err != nil {
		return "", err
	}

	if err := s.txRequests.push(*request, boardingInputs); err != nil {
		return "", err
	}
	return request.Id, nil
}

func (s *covenantlessService) newBoardingInput(tx wire.MsgTx, input ports.Input) (*ports.BoardingInput, error) {
	if len(tx.TxOut) <= int(input.VtxoKey.VOut) {
		return nil, fmt.Errorf("output not found")
	}

	output := tx.TxOut[input.VtxoKey.VOut]

	boardingScript, err := bitcointree.ParseVtxoScript(input.Tapscripts)
	if err != nil {
		return nil, fmt.Errorf("failed to parse boarding descriptor: %s", err)
	}

	tapKey, _, err := boardingScript.TapTree()
	if err != nil {
		return nil, fmt.Errorf("failed to get taproot key: %s", err)
	}

	expectedScriptPubkey, err := common.P2TRScript(tapKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get script pubkey: %s", err)
	}

	if !bytes.Equal(output.PkScript, expectedScriptPubkey) {
		return nil, fmt.Errorf("descriptor does not match script in transaction output")
	}

	if err := boardingScript.Validate(s.pubkey, s.unilateralExitDelay); err != nil {
		return nil, err
	}

	return &ports.BoardingInput{
		Amount: uint64(output.Value),
		Input:  input,
	}, nil
}

func (s *covenantlessService) ClaimVtxos(ctx context.Context, creds string, receivers []domain.Receiver, musig2Data *tree.Musig2) error {
	// Check credentials
	request, ok := s.txRequests.view(creds)
	if !ok {
		return fmt.Errorf("invalid credentials")
	}

	dustAmount, err := s.wallet.GetDustAmount(ctx)
	if err != nil {
		return fmt.Errorf("unable to verify receiver amount, failed to get dust: %s", err)
	}

	hasOffChainReceiver := false

	for _, rcv := range receivers {
		if rcv.Amount <= dustAmount {
			return fmt.Errorf("receiver amount must be greater than dust amount %d", dustAmount)
		}

		if !rcv.IsOnchain() {
			hasOffChainReceiver = true
		}
	}

	var data *tree.Musig2

	if hasOffChainReceiver {
		if musig2Data == nil {
			return fmt.Errorf("musig2 data is required for offchain receivers")
		}

		// check if the server pubkey has been set as cosigner
		serverPubKeyHex := hex.EncodeToString(s.serverSigningPubKey.SerializeCompressed())
		for _, pubkey := range musig2Data.CosignersPublicKeys {
			if pubkey == serverPubKeyHex {
				return fmt.Errorf("server pubkey already in musig2 data")
			}
		}

		data = musig2Data
	}

	if err := request.AddReceivers(receivers); err != nil {
		return err
	}

	return s.txRequests.update(*request, data)
}

func (s *covenantlessService) UpdateTxRequestStatus(_ context.Context, id string) error {
	return s.txRequests.updatePingTimestamp(id)
}

func (s *covenantlessService) SignVtxos(ctx context.Context, forfeitTxs []string) error {
	if forfeitTxs == nil || len(forfeitTxs) == 0 {
		return fmt.Errorf("no forfeit transactions provided")
	}

	// Validate all forfeit transactions before signing any of them
	for i, txHex := range forfeitTxs {
		if txHex == "" {
			return fmt.Errorf("forfeit transaction %d is empty", i)
		}
		
		// Verify basic transaction structure before signing
		if valid, txid, err := s.builder.ValidateTransaction(txHex); err != nil {
			return fmt.Errorf("failed to validate forfeit tx %d: %w", i, err)
		} else if !valid {
			return fmt.Errorf("forfeit tx %d is invalid (txid: %s)", i, txid)
		}
	}
	
	// Use a mutex to protect access to the forfeitTxs map
	if err := s.forfeitTxs.sign(forfeitTxs); err != nil {
		return fmt.Errorf("failed to sign forfeit transactions: %w", err)
	}

	// Clone the current round to avoid race conditions
	s.currentRoundLock.Lock()
	round := s.currentRound
	s.currentRoundLock.Unlock()
	
	if round == nil {
		return fmt.Errorf("no active round available for signing")
	}
	
	// Process signatures in a separate goroutine to avoid blocking
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("recovered from panic in checkForfeitsAndBoardingSigsSent: %v", r)
				debug.PrintStack()
			}
		}()
		
		s.checkForfeitsAndBoardingSigsSent(round)
	}()

	return nil
}

func (s *covenantlessService) SignRoundTx(ctx context.Context, signedRoundTx string) error {
	if signedRoundTx == "" {
		return fmt.Errorf("signed round transaction is empty")
	}

	// Validate the transaction structure before proceeding
	if valid, txid, err := s.builder.ValidateTransaction(signedRoundTx); err != nil {
		return fmt.Errorf("failed to validate round transaction: %w", err)
	} else if !valid {
		return fmt.Errorf("invalid round transaction (txid: %s)", txid)
	}
	
	// Make sure we have a current round
	s.currentRoundLock.Lock()
	if s.currentRound == nil {
		s.currentRoundLock.Unlock()
		return fmt.Errorf("no active round available for signing")
	}
	
	// Verify and combine signatures
	currentRound := s.currentRound
	
	// Verify signature against expected transaction format
	combined, err := s.builder.VerifyAndCombinePartialTx(currentRound.UnsignedTx, signedRoundTx)
	if err != nil {
		s.currentRoundLock.Unlock()
		return fmt.Errorf("failed to verify and combine partial tx: %w", err)
	}

	// Update the transaction with the combined signatures
	s.currentRound.UnsignedTx = combined
	s.currentRoundLock.Unlock()

	// Process in a separate goroutine to avoid blocking
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("recovered from panic in checkForfeitsAndBoardingSigsSent after SignRoundTx: %v", r)
				debug.PrintStack()
			}
		}()
		
		// Safely get the current round
		s.currentRoundLock.Lock()
		round := s.currentRound
		s.currentRoundLock.Unlock()
		
		if round != nil {
			s.checkForfeitsAndBoardingSigsSent(round)
		}
	}()

	return nil
}

func (s *covenantlessService) checkForfeitsAndBoardingSigsSent(currentRound *domain.Round) {
	if currentRound == nil {
		log.Error().Msg("checkForfeitsAndBoardingSigsSent called with nil round")
		return
	}
	
	if currentRound.UnsignedTx == "" {
		log.Error().Str("round_id", currentRound.Id).Msg("Round has empty UnsignedTx")
		return
	}
	
	// Parse the transaction to check signatures
	roundTx, err := psbt.NewFromRawBytes(strings.NewReader(currentRound.UnsignedTx), true)
	if err != nil {
		log.Error().
			Err(err).
			Str("round_id", currentRound.Id).
			Msg("Failed to parse round transaction")
		return
	}
	
	// Count signed inputs to verify boarding inputs are signed
	numOfInputsSigned := 0
	for i, v := range roundTx.Inputs {
		if len(v.TaprootScriptSpendSig) > 0 {
			if len(v.TaprootScriptSpendSig[0].Signature) > 0 {
				numOfInputsSigned++
				log.Debug().
					Str("round_id", currentRound.Id).
					Int("input_index", i).
					Msg("Input is signed")
			} else {
				log.Debug().
					Str("round_id", currentRound.Id).
					Int("input_index", i).
					Msg("Input has empty signature")
			}
		} else {
			log.Debug().
				Str("round_id", currentRound.Id).
				Int("input_index", i).
				Msg("Input has no TaprootScriptSpendSig")
		}
	}

	// Create a mutex to protect this operation
	sigCheckMutex := &sync.Mutex{}
	sigCheckMutex.Lock()
	defer sigCheckMutex.Unlock()
	
	// Get the current number of expected boarding inputs
	s.numOfBoardingInputsMtx.RLock()
	numOfBoardingInputs := s.numOfBoardingInputs
	s.numOfBoardingInputsMtx.RUnlock()
	
	// Check if all forfeit transactions are signed
	allForfeitsSigned := s.forfeitTxs.allSigned()
	
	log.Debug().
		Str("round_id", currentRound.Id).
		Int("num_signed_inputs", numOfInputsSigned).
		Int("expected_boarding_inputs", numOfBoardingInputs).
		Bool("all_forfeits_signed", allForfeitsSigned).
		Msg("Checking signature completion status")
	
	// Check completion condition:
	// 1. All forfeit transactions must be signed
	// 2. The number of signed boarding inputs must match the expected count
	if allForfeitsSigned && numOfBoardingInputs == numOfInputsSigned {
		log.Info().
			Str("round_id", currentRound.Id).
			Msg("All signatures collected, proceeding with round")
			
		// Signal completion non-blocking
		select {
		case s.forfeitsBoardingSigsChan <- struct{}{}:
			log.Debug().Str("round_id", currentRound.Id).Msg("Sent forfeit signature completion signal")
		default:
			log.Debug().Str("round_id", currentRound.Id).Msg("Channel already has completion signal")
		}
	} else {
		log.Debug().
			Str("round_id", currentRound.Id).
			Bool("forfeits_signed", allForfeitsSigned).
			Int("signed_inputs", numOfInputsSigned).
			Int("expected_inputs", numOfBoardingInputs).
			Msg("Not all signatures are collected yet")
	}
}

func (s *covenantlessService) ListVtxos(ctx context.Context, address string) ([]domain.Vtxo, []domain.Vtxo, error) {
	decodedAddress, err := common.DecodeAddress(address)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode address: %s", err)
	}

	if !bytes.Equal(schnorr.SerializePubKey(decodedAddress.Server), schnorr.SerializePubKey(s.pubkey)) {
		return nil, nil, fmt.Errorf("address does not match server pubkey")
	}

	pubkey := hex.EncodeToString(schnorr.SerializePubKey(decodedAddress.VtxoTapKey))

	return s.repoManager.Vtxos().GetAllVtxos(ctx, pubkey)
}

func (s *covenantlessService) GetEventsChannel(ctx context.Context) <-chan domain.RoundEvent {
	return s.eventsCh
}

func (s *covenantlessService) GetTransactionEventsChannel(ctx context.Context) <-chan TransactionEvent {
	return s.transactionEventsCh
}

func (s *covenantlessService) GetRoundByTxid(ctx context.Context, roundTxid string) (*domain.Round, error) {
	return s.repoManager.Rounds().GetRoundWithTxid(ctx, roundTxid)
}

func (s *covenantlessService) GetRoundById(ctx context.Context, id string) (*domain.Round, error) {
	return s.repoManager.Rounds().GetRoundWithId(ctx, id)
}

func (s *covenantlessService) GetCurrentRound(ctx context.Context) (*domain.Round, error) {
	return domain.NewRoundFromEvents(s.currentRound.Events()), nil
}

func (s *covenantlessService) GetInfo(ctx context.Context) (*ServiceInfo, error) {
	pubkey := hex.EncodeToString(s.pubkey.SerializeCompressed())

	dust, err := s.wallet.GetDustAmount(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get dust amount: %s", err)
	}

	forfeitAddr, err := s.wallet.GetForfeitAddress(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get forfeit address: %s", err)
	}

	marketHourConfig, err := s.repoManager.MarketHourRepo().Get(ctx)
	if err != nil {
		return nil, err
	}

	marketHourNextStart, marketHourNextEnd, err := calcNextMarketHour(
		marketHourConfig.StartTime,
		marketHourConfig.EndTime,
		marketHourConfig.Period,
		marketHourDelta,
		time.Now(),
	)
	if err != nil {
		return nil, err
	}

	return &ServiceInfo{
		PubKey:              pubkey,
		VtxoTreeExpiry:      int64(s.vtxoTreeExpiry.Value),
		UnilateralExitDelay: int64(s.unilateralExitDelay.Value),
		RoundInterval:       s.roundInterval,
		Network:             s.network.Name,
		Dust:                dust,
		ForfeitAddress:      forfeitAddr,
		NextMarketHour: &NextMarketHour{
			StartTime:     marketHourNextStart,
			EndTime:       marketHourNextEnd,
			Period:        marketHourConfig.Period,
			RoundInterval: marketHourConfig.RoundInterval,
		},
	}, nil
}

func (s *covenantlessService) GetTxRequestQueue(
	ctx context.Context, requestIds ...string,
) ([]TxRequestInfo, error) {
	requests, err := s.txRequests.viewAll(requestIds)
	if err != nil {
		return nil, err
	}

	txReqsInfo := make([]TxRequestInfo, 0, len(requests))
	for _, request := range requests {
		signingType := "branch"
		cosigners := make([]string, 0)
		if request.musig2Data != nil {
			if request.musig2Data.SigningType == tree.SignAll {
				signingType = "all"
			}
			cosigners = request.musig2Data.CosignersPublicKeys
		}

		receivers := make([]struct {
			Address string
			Amount  uint64
		}, 0, len(request.Receivers))
		for _, receiver := range request.Receivers {
			if len(receiver.OnchainAddress) > 0 {
				receivers = append(receivers, struct {
					Address string
					Amount  uint64
				}{
					Address: receiver.OnchainAddress,
					Amount:  receiver.Amount,
				})
				continue
			}

			pubkey, err := hex.DecodeString(receiver.PubKey)
			if err != nil {
				return nil, fmt.Errorf("failed to decode pubkey: %s", err)
			}

			vtxoTapKey, err := schnorr.ParsePubKey(pubkey)
			if err != nil {
				return nil, fmt.Errorf("failed to parse pubkey: %s", err)
			}

			address := common.Address{
				HRP:        s.network.Addr,
				Server:     s.pubkey,
				VtxoTapKey: vtxoTapKey,
			}

			addressStr, err := address.Encode()
			if err != nil {
				return nil, fmt.Errorf("failed to encode address: %s", err)
			}

			receivers = append(receivers, struct {
				Address string
				Amount  uint64
			}{
				Address: addressStr,
				Amount:  receiver.Amount,
			})
		}

		txReqsInfo = append(txReqsInfo, TxRequestInfo{
			Id:             request.Id,
			CreatedAt:      request.timestamp,
			Receivers:      receivers,
			Inputs:         request.Inputs,
			BoardingInputs: request.boardingInputs,
			Notes:          request.notes,
			LastPing:       request.pingTimestamp,
			SigningType:    signingType,
			Cosigners:      cosigners,
		})
	}

	return txReqsInfo, nil
}

func (s *covenantlessService) DeleteTxRequests(
	ctx context.Context, requestIds ...string,
) error {
	if len(requestIds) == 0 {
		return s.txRequests.deleteAll()
	}

	return s.txRequests.delete(requestIds)
}

func calcNextMarketHour(marketHourStartTime, marketHourEndTime time.Time, period, marketHourDelta time.Duration, now time.Time) (time.Time, time.Time, error) {
	// Validate input parameters
	if period <= 0 {
		return time.Time{}, time.Time{}, fmt.Errorf("period must be greater than 0")
	}
	if !marketHourEndTime.After(marketHourStartTime) {
		return time.Time{}, time.Time{}, fmt.Errorf("market hour end time must be after start time")
	}

	// Calculate the duration of the market hour
	duration := marketHourEndTime.Sub(marketHourStartTime)

	// Calculate the number of periods since the initial marketHourStartTime
	elapsed := now.Sub(marketHourStartTime)
	var n int64
	if elapsed >= 0 {
		n = int64(elapsed / period)
	} else {
		n = int64((elapsed - period + 1) / period)
	}

	// Calculate the current market hour start and end times
	currentStartTime := marketHourStartTime.Add(time.Duration(n) * period)
	currentEndTime := currentStartTime.Add(duration)

	// Adjust if now is before the currentStartTime
	if now.Before(currentStartTime) {
		n -= 1
		currentStartTime = marketHourStartTime.Add(time.Duration(n) * period)
		currentEndTime = currentStartTime.Add(duration)
	}

	timeUntilEnd := currentEndTime.Sub(now)

	if !now.Before(currentStartTime) && now.Before(currentEndTime) && timeUntilEnd >= marketHourDelta {
		// Return the current market hour
		return currentStartTime, currentEndTime, nil
	} else {
		// Move to the next market hour
		n += 1
		nextStartTime := marketHourStartTime.Add(time.Duration(n) * period)
		nextEndTime := nextStartTime.Add(duration)
		return nextStartTime, nextEndTime, nil
	}
}

func (s *covenantlessService) RegisterCosignerNonces(
	ctx context.Context, roundID string, pubkey *secp256k1.PublicKey, encodedNonces string,
) error {
	session, ok := s.treeSigningSessions[roundID]
	if ok {
		// Update last used time for this session
		s.signingSessionsLock.Lock()
		s.signingSessionsLastUsed[roundID] = time.Now()
		s.signingSessionsLock.Unlock()
	} else {
		return fmt.Errorf(`signing session not found for round "%s"`, roundID)
	}

	userPubkey := hex.EncodeToString(pubkey.SerializeCompressed())
	if _, ok := session.cosigners[userPubkey]; !ok {
		return fmt.Errorf(`cosigner %s not found for round "%s"`, userPubkey, roundID)
	}

	nonces, err := bitcointree.DecodeNonces(hex.NewDecoder(strings.NewReader(encodedNonces)))
	if err != nil {
		return fmt.Errorf("failed to decode nonces: %s", err)
	}
	session.lock.Lock()
	defer session.lock.Unlock()

	if _, ok := session.nonces[pubkey]; ok {
		return nil // skip if we already have nonces for this pubkey
	}

	session.nonces[pubkey] = nonces

	if len(session.nonces) == session.nbCosigners-1 { // exclude the server
		go func() {
			session.nonceDoneC <- struct{}{}
		}()
	}

	return nil
}

func (s *covenantlessService) RegisterCosignerSignatures(
	ctx context.Context, roundID string, pubkey *secp256k1.PublicKey, encodedSignatures string,
) error {
	session, ok := s.treeSigningSessions[roundID]
	if ok {
		// Update last used time for this session
		s.signingSessionsLock.Lock()
		s.signingSessionsLastUsed[roundID] = time.Now()
		s.signingSessionsLock.Unlock()
	} else {
		return fmt.Errorf(`signing session not found for round "%s"`, roundID)
	}

	userPubkey := hex.EncodeToString(pubkey.SerializeCompressed())
	if _, ok := session.cosigners[userPubkey]; !ok {
		return fmt.Errorf(`cosigner %s not found for round "%s"`, userPubkey, roundID)
	}

	signatures, err := bitcointree.DecodeSignatures(hex.NewDecoder(strings.NewReader(encodedSignatures)))
	if err != nil {
		return fmt.Errorf("failed to decode signatures: %s", err)
	}

	session.lock.Lock()
	defer session.lock.Unlock()

	if _, ok := session.signatures[pubkey]; ok {
		return nil // skip if we already have signatures for this pubkey
	}

	session.signatures[pubkey] = signatures

	if len(session.signatures) == session.nbCosigners-1 { // exclude the server
		go func() {
			session.sigDoneC <- struct{}{}
		}()
	}

	return nil
}

func (s *covenantlessService) SetNostrRecipient(ctx context.Context, nostrRecipient string, signedVtxoOutpoints []SignedVtxoOutpoint) error {
	nprofileRecipient, err := nip19toNostrProfile(nostrRecipient, s.nostrDefaultRelays)
	if err != nil {
		return fmt.Errorf("failed to convert nostr recipient: %s", err)
	}

	if err := validateProofs(ctx, s.repoManager.Vtxos(), signedVtxoOutpoints); err != nil {
		return err
	}

	vtxoKeys := make([]domain.VtxoKey, 0, len(signedVtxoOutpoints))
	for _, signedVtxo := range signedVtxoOutpoints {
		vtxoKeys = append(vtxoKeys, signedVtxo.Outpoint)
	}

	return s.repoManager.Entities().Add(
		ctx,
		domain.Entity{
			NostrRecipient: nprofileRecipient,
		},
		vtxoKeys,
	)
}

func (s *covenantlessService) DeleteNostrRecipient(ctx context.Context, signedVtxoOutpoints []SignedVtxoOutpoint) error {
	if err := validateProofs(ctx, s.repoManager.Vtxos(), signedVtxoOutpoints); err != nil {
		return err
	}

	vtxoKeys := make([]domain.VtxoKey, 0, len(signedVtxoOutpoints))
	for _, signedVtxo := range signedVtxoOutpoints {
		vtxoKeys = append(vtxoKeys, signedVtxo.Outpoint)
	}

	return s.repoManager.Entities().Delete(ctx, vtxoKeys)
}

func (s *covenantlessService) start() {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("recovered from panic in start: %v", r)
		}
	}()

	s.startRound()
}

func (s *covenantlessService) startRound() {
	// reset the forfeit txs map to avoid polluting the next batch of forfeits transactions
	s.forfeitTxs.reset()

	dustAmount, err := s.wallet.GetDustAmount(context.Background())
	if err != nil {
		log.WithError(err).Warn("failed to get dust amount")
		return
	}

	round := domain.NewRound(dustAmount)
	//nolint:all
	round.StartRegistration()
	s.currentRound = round
	close(s.forfeitsBoardingSigsChan)
	s.forfeitsBoardingSigsChan = make(chan struct{}, 1)

	defer func() {
		roundEndTime := time.Now().Add(time.Duration(s.roundInterval) * time.Second)
		sleepingTime := s.roundInterval / 6
		if sleepingTime < 1 {
			sleepingTime = 1
		}
		time.Sleep(time.Duration(sleepingTime) * time.Second)
		s.startFinalization(roundEndTime)
	}()

	log.Debugf("started registration stage for new round: %s", round.Id)
}
func (s *covenantlessService) startFinalization(roundEndTime time.Time) {
	log.Debugf("started finalization stage for round: %s", s.currentRound.Id)
	ctx := context.Background()
	round := s.currentRound

	roundRemainingDuration := time.Duration((s.roundInterval/3)*2-1) * time.Second
	thirdOfRemainingDuration := roundRemainingDuration / 3

	var notes []note.Note
	var roundAborted bool
	defer func() {
		// Clean up signing session and its last used timestamp
		delete(s.treeSigningSessions, round.Id)
		s.signingSessionsLock.Lock()
		delete(s.signingSessionsLastUsed, round.Id)
		s.signingSessionsLock.Unlock()
		if roundAborted {
			s.startRound()
			return
		}

		if err := s.saveEvents(ctx, round.Id, round.Events()); err != nil {
			log.WithError(err).Warn("failed to store new round events")
		}

		if round.IsFailed() {
			s.startRound()
			return
		}

		s.finalizeRound(notes, roundEndTime)
	}()

	if round.IsFailed() {
		return
	}

	// TODO: understand how many tx requests must be popped from the queue and actually registered for the round
	num := s.txRequests.len()
	if num == 0 {
		roundAborted = true
		err := fmt.Errorf("no tx requests registered")
		round.Fail(fmt.Errorf("round aborted: %s", err))
		log.WithError(err).Debugf("round %s aborted", round.Id)
		return
	}
	if num > txRequestsThreshold {
		num = txRequestsThreshold
	}
	requests, boardingInputs, redeeemedNotes, musig2data := s.txRequests.pop(num)
	notes = redeeemedNotes
	s.numOfBoardingInputsMtx.Lock()
	s.numOfBoardingInputs = len(boardingInputs)
	s.numOfBoardingInputsMtx.Unlock()

	if _, err := round.RegisterTxRequests(requests); err != nil {
		round.Fail(fmt.Errorf("failed to register tx requests: %s", err))
		log.WithError(err).Warn("failed to register tx requests")
		return
	}

	connectorAddresses, err := s.repoManager.Rounds().GetSweptRoundsConnectorAddress(ctx)
	if err != nil {
		round.Fail(fmt.Errorf("failed to retrieve swept rounds: %s", err))
		log.WithError(err).Warn("failed to retrieve swept rounds")
		return
	}

	// add server pubkey in musig2data and count the number of unique keys
	uniqueSignerPubkeys := make(map[string]struct{})
	serverPubKeyHex := hex.EncodeToString(s.serverSigningPubKey.SerializeCompressed())
	for _, data := range musig2data {
		if data == nil {
			continue
		}
		for _, pubkey := range data.CosignersPublicKeys {
			uniqueSignerPubkeys[pubkey] = struct{}{}
		}
		data.CosignersPublicKeys = append(data.CosignersPublicKeys, serverPubKeyHex)
	}
	log.Debugf("building tx for round %s", round.Id)
	unsignedRoundTx, vtxoTree, connectorAddress, connectors, err := s.builder.BuildRoundTx(
		s.pubkey, requests, boardingInputs, connectorAddresses, musig2data,
	)
	if err != nil {
		round.Fail(fmt.Errorf("failed to create round tx: %s", err))
		log.WithError(err).Warn("failed to create round tx")
		return
	}
	log.Debugf("round tx created for round %s", round.Id)

	if err := s.forfeitTxs.init(connectors, requests); err != nil {
		round.Fail(fmt.Errorf("failed to initialize forfeit txs: %s", err))
		log.WithError(err).Warn("failed to initialize forfeit txs")
		return
	}

	if len(vtxoTree) > 0 {
		sweepClosure := tree.CSVMultisigClosure{
			MultisigClosure: tree.MultisigClosure{PubKeys: []*secp256k1.PublicKey{s.pubkey}},
			Locktime:        s.vtxoTreeExpiry,
		}

		sweepScript, err := sweepClosure.Script()
		if err != nil {
			return
		}

		unsignedPsbt, err := psbt.NewFromRawBytes(strings.NewReader(unsignedRoundTx), true)
		if err != nil {
			round.Fail(fmt.Errorf("failed to parse round tx: %s", err))
			log.WithError(err).Warn("failed to parse round tx")
			return
		}

		sharedOutputAmount := unsignedPsbt.UnsignedTx.TxOut[0].Value

		sweepLeaf := txscript.NewBaseTapLeaf(sweepScript)
		sweepTapTree := txscript.AssembleTaprootScriptTree(sweepLeaf)
		root := sweepTapTree.RootNode.TapHash()

		coordinator, err := bitcointree.NewTreeCoordinatorSession(sharedOutputAmount, vtxoTree, root.CloneBytes())
		if err != nil {
			round.Fail(fmt.Errorf("failed to create tree coordinator: %s", err))
			log.WithError(err).Warn("failed to create tree coordinator")
			return
		}

		serverSignerSession := bitcointree.NewTreeSignerSession(s.serverSigningKey)
		if err := serverSignerSession.Init(root.CloneBytes(), sharedOutputAmount, vtxoTree); err != nil {
			round.Fail(fmt.Errorf("failed to create tree signer session: %s", err))
			log.WithError(err).Warn("failed to create tree signer session")
			return
		}

		nonces, err := serverSignerSession.GetNonces()
		if err != nil {
			round.Fail(fmt.Errorf("failed to get nonces: %s", err))
			log.WithError(err).Warn("failed to get nonces")
			return
		}

		coordinator.AddNonce(s.serverSigningPubKey, nonces)

		signingSession := newMusigSigningSession(uniqueSignerPubkeys)
		
		// Add to signing sessions with timestamp
		s.treeSigningSessions[round.Id] = signingSession
		s.signingSessionsLock.Lock()
		s.signingSessionsLastUsed[round.Id] = time.Now()
		s.signingSessionsLock.Unlock()

		log.Debugf("signing session created for round %s with %d signers", round.Id, len(uniqueSignerPubkeys))

		s.currentRound.UnsignedTx = unsignedRoundTx
		// send back the unsigned tree & all cosigners pubkeys
		listOfCosignersPubkeys := make([]string, 0, len(uniqueSignerPubkeys))
		for pubkey := range uniqueSignerPubkeys {
			listOfCosignersPubkeys = append(listOfCosignersPubkeys, pubkey)
		}

		s.propagateRoundSigningStartedEvent(vtxoTree, listOfCosignersPubkeys)

		noncesTimer := time.NewTimer(thirdOfRemainingDuration)

		select {
		case <-noncesTimer.C:
			err := fmt.Errorf(
				"musig2 signing session timed out (nonce collection), collected %d/%d nonces",
				len(signingSession.nonces), len(uniqueSignerPubkeys),
			)
			round.Fail(err)
			log.Warn(err)
			return
		case <-signingSession.nonceDoneC:
			noncesTimer.Stop()
			for pubkey, nonce := range signingSession.nonces {
				coordinator.AddNonce(pubkey, nonce)
			}
		}

		log.Debugf("nonces collected for round %s", round.Id)

		aggregatedNonces, err := coordinator.AggregateNonces()
		if err != nil {
			round.Fail(fmt.Errorf("failed to aggregate nonces: %s", err))
			log.WithError(err).Warn("failed to aggregate nonces")
			return
		}

		log.Debugf("nonces aggregated for round %s", round.Id)

		serverSignerSession.SetAggregatedNonces(aggregatedNonces)

		// send the combined nonces to the clients
		s.propagateRoundSigningNoncesGeneratedEvent(aggregatedNonces)

		// sign the tree as server
		serverTreeSigs, err := serverSignerSession.Sign()
		if err != nil {
			round.Fail(fmt.Errorf("failed to sign tree: %s", err))
			log.WithError(err).Warn("failed to sign tree")
			return
		}
		coordinator.AddSignatures(s.serverSigningPubKey, serverTreeSigs)

		log.Debugf("tree signed by us for round %s", round.Id)

		signaturesTimer := time.NewTimer(thirdOfRemainingDuration)

		log.Debugf("waiting for cosigners to sign the tree")

		select {
		case <-signaturesTimer.C:
			err := fmt.Errorf(
				"musig2 signing session timed out (signatures collection), collected %d/%d signatures",
				len(signingSession.signatures), len(uniqueSignerPubkeys),
			)
			round.Fail(err)
			log.Warn(err)
			return
		case <-signingSession.sigDoneC:
			signaturesTimer.Stop()
			for pubkey, sig := range signingSession.signatures {
				coordinator.AddSignatures(pubkey, sig)
			}
		}

		log.Debugf("signatures collected for round %s", round.Id)

		signedTree, err := coordinator.SignTree()
		if err != nil {
			round.Fail(fmt.Errorf("failed to aggregate tree signatures: %s", err))
			log.WithError(err).Warn("failed to aggregate tree signatures")
			return
		}

		log.Debugf("vtxo tree signed for round %s", round.Id)

		vtxoTree = signedTree
	}

	_, err = round.StartFinalization(
		connectorAddress, connectors, vtxoTree, unsignedRoundTx, s.forfeitTxs.connectorsIndex,
	)
	if err != nil {
		round.Fail(fmt.Errorf("failed to start finalization: %s", err))
		log.WithError(err).Warn("failed to start finalization")
		return
	}

	log.Debugf("started finalization stage for round: %s", round.Id)
}

func (s *covenantlessService) propagateRoundSigningStartedEvent(unsignedVtxoTree tree.TxTree, cosignersPubkeys []string) {
	ev := RoundSigningStarted{
		Id:               s.currentRound.Id,
		UnsignedVtxoTree: unsignedVtxoTree,
		UnsignedRoundTx:  s.currentRound.UnsignedTx,
		CosignersPubkeys: cosignersPubkeys,
	}

	s.eventsCh <- ev
}

func (s *covenantlessService) propagateRoundSigningNoncesGeneratedEvent(combinedNonces bitcointree.TreeNonces) {
	ev := RoundSigningNoncesGenerated{
		Id:     s.currentRound.Id,
		Nonces: combinedNonces,
	}

	s.eventsCh <- ev
}

func (s *covenantlessService) finalizeRound(notes []note.Note, roundEndTime time.Time) {
	defer s.startRound()

	ctx := context.Background()
	s.currentRoundLock.Lock()
	round := s.currentRound
	s.currentRoundLock.Unlock()
	if round.IsFailed() {
		return
	}

	var changes []domain.RoundEvent
	defer func() {
		if err := s.saveEvents(ctx, round.Id, changes); err != nil {
			log.WithError(err).Warn("failed to store new round events")
			return
		}
	}()

	remainingTime := time.Until(roundEndTime)
	select {
	case <-s.forfeitsBoardingSigsChan:
		log.Debug("all forfeit txs and boarding inputs signatures have been sent")
	case <-time.After(remainingTime):
		log.Debug("timeout waiting for forfeit txs and boarding inputs signatures")
	}

	forfeitTxs, err := s.forfeitTxs.pop()
	if err != nil {
		changes = round.Fail(fmt.Errorf("failed to finalize round: %s", err))
		log.WithError(err).Warn("failed to finalize round")
		return
	}

	if err := s.verifyForfeitTxsSigs(forfeitTxs); err != nil {
		changes = round.Fail(err)
		log.WithError(err).Warn("failed to validate forfeit txs")
		return
	}

	log.Debugf("signing round transaction %s\n", round.Id)

	boardingInputsIndexes := make([]int, 0)
	boardingInputs := make([]domain.VtxoKey, 0)
	roundTx, err := psbt.NewFromRawBytes(strings.NewReader(round.UnsignedTx), true)
	if err != nil {
		log.Debugf("failed to parse round tx: %s", round.UnsignedTx)
		changes = round.Fail(fmt.Errorf("failed to parse round tx: %s", err))
		log.WithError(err).Warn("failed to parse round tx")
		return
	}

	for i, in := range roundTx.Inputs {
		if len(in.TaprootLeafScript) > 0 {
			if len(in.TaprootScriptSpendSig) == 0 {
				err = fmt.Errorf("missing tapscript spend sig for input %d", i)
				changes = round.Fail(err)
				log.WithError(err).Warn("missing boarding sig")
				return
			}

			boardingInputsIndexes = append(boardingInputsIndexes, i)
			boardingInputs = append(boardingInputs, domain.VtxoKey{
				Txid: roundTx.UnsignedTx.TxIn[i].PreviousOutPoint.Hash.String(),
				VOut: roundTx.UnsignedTx.TxIn[i].PreviousOutPoint.Index,
			})
		}
	}

	signedRoundTx := round.UnsignedTx

	if len(boardingInputsIndexes) > 0 {
		signedRoundTx, err = s.wallet.SignTransactionTapscript(ctx, signedRoundTx, boardingInputsIndexes)
		if err != nil {
			changes = round.Fail(fmt.Errorf("failed to sign round tx: %s", err))
			log.WithError(err).Warn("failed to sign round tx")
			return
		}
	}

	signedRoundTx, err = s.wallet.SignTransaction(ctx, signedRoundTx, true)
	if err != nil {
		changes = round.Fail(fmt.Errorf("failed to sign round tx: %s", err))
		log.WithError(err).Warn("failed to sign round tx")
		return
	}

	txid, err := s.wallet.BroadcastTransaction(ctx, signedRoundTx)
	if err != nil {
		changes = round.Fail(fmt.Errorf("failed to broadcast round tx: %s", err))
		return
	}

	changes, err = round.EndFinalization(forfeitTxs, txid)
	if err != nil {
		changes = round.Fail(fmt.Errorf("failed to finalize round: %s", err))
		log.WithError(err).Warn("failed to finalize round")
		return
	}

	// mark the notes as spent
	for _, note := range notes {
		if err := s.repoManager.Notes().Add(ctx, note.ID); err != nil {
			log.WithError(err).Warn("failed to mark note as spent")
		}
	}

	go func() {
		s.transactionEventsCh <- RoundTransactionEvent{
			RoundTxid:             round.Txid,
			SpentVtxos:            s.getSpentVtxos(round.TxRequests),
			SpendableVtxos:        s.getNewVtxos(round),
			ClaimedBoardingInputs: boardingInputs,
		}
	}()

	log.Debugf("finalized round %s with round tx %s", round.Id, round.Txid)
}

func (s *covenantlessService) listenToScannerNotifications() {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("recovered from panic in listenToScannerNotifications: %v", r)
		}
	}()

	ctx := context.Background()
	chVtxos := s.scanner.GetNotificationChannel(ctx)

	mutx := &sync.Mutex{}
	for vtxoKeys := range chVtxos {
		go func(vtxoKeys map[string][]ports.VtxoWithValue) {
			defer func() {
				if r := recover(); r != nil {
					log.Errorf("recovered from panic in GetVtxos: %v", r)
				}
			}()

			for _, keys := range vtxoKeys {
				for _, v := range keys {
					vtxos, err := s.repoManager.Vtxos().GetVtxos(ctx, []domain.VtxoKey{v.VtxoKey})
					if err != nil {
						log.WithError(err).Warn("failed to retrieve vtxos, skipping...")
						return
					}
					vtxo := vtxos[0]

					if !vtxo.Redeemed {
						go func() {
							defer func() {
								if r := recover(); r != nil {
									log.Errorf("recovered from panic in markAsRedeemed: %v", r)
								}
							}()

							if err := s.markAsRedeemed(ctx, vtxo); err != nil {
								log.WithError(err).Warnf("failed to mark vtxo %s:%d as redeemed", vtxo.Txid, vtxo.VOut)
							}
						}()
					}

					if vtxo.Spent {
						log.Infof("fraud detected on vtxo %s:%d", vtxo.Txid, vtxo.VOut)
						go func() {
							defer func() {
								if r := recover(); r != nil {
									log.Errorf("recovered from panic in reactToFraud: %v", r)
									// log the stack trace
									log.Errorf("stack trace: %s", string(debug.Stack()))
								}
							}()

							if err := s.reactToFraud(ctx, vtxo, mutx); err != nil {
								log.WithError(err).Warnf("failed to prevent fraud for vtxo %s:%d", vtxo.Txid, vtxo.VOut)
							}
						}()
					}
				}
			}
		}(vtxoKeys)
	}
}

func (s *covenantlessService) updateVtxoSet(round *domain.Round) {
	// Update the vtxo set only after a round is finalized.
	if !round.IsEnded() {
		return
	}

	ctx := context.Background()
	repo := s.repoManager.Vtxos()
	spentVtxos := getSpentVtxos(round.TxRequests)
	if len(spentVtxos) > 0 {
		for {
			if err := repo.SpendVtxos(ctx, spentVtxos, round.Txid); err != nil {
				log.WithError(err).Warn("failed to add new vtxos, retrying soon")
				time.Sleep(100 * time.Millisecond)
				continue
			}
			log.Debugf("spent %d vtxos", len(spentVtxos))
			break
		}
	}

	newVtxos := s.getNewVtxos(round)
	if len(newVtxos) > 0 {
		for {
			if err := repo.AddVtxos(ctx, newVtxos); err != nil {
				log.WithError(err).Warn("failed to add new vtxos, retrying soon")
				time.Sleep(100 * time.Millisecond)
				continue
			}
			log.Debugf("added %d new vtxos", len(newVtxos))
			break
		}

		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Errorf("recovered from panic in startWatchingVtxos: %v", r)
				}
			}()

			for {
				if err := s.startWatchingVtxos(newVtxos); err != nil {
					log.WithError(err).Warn(
						"failed to start watching vtxos, retrying in a moment...",
					)
					continue
				}
				log.Debugf("started watching %d vtxos", len(newVtxos))
				return
			}
		}()

	}
}

func (s *covenantlessService) propagateEvents(round *domain.Round) {
	lastEvent := round.Events()[len(round.Events())-1]
	switch e := lastEvent.(type) {
	case domain.RoundFinalizationStarted:
		ev := domain.RoundFinalizationStarted{
			Id:               e.Id,
			VtxoTree:         e.VtxoTree,
			Connectors:       e.Connectors,
			RoundTx:          e.RoundTx,
			MinRelayFeeRate:  int64(s.wallet.MinRelayFeeRate(context.Background())),
			ConnectorAddress: e.ConnectorAddress,
			ConnectorsIndex:  e.ConnectorsIndex,
		}
		s.eventsCh <- ev
	case domain.RoundFinalized, domain.RoundFailed:
		s.eventsCh <- e
	}
}

func (s *covenantlessService) scheduleSweepVtxosForRound(round *domain.Round) {
	// Schedule the sweeping procedure only for completed round.
	if !round.IsEnded() {
		return
	}

	expirationTimestamp := s.sweeper.scheduler.AddNow(int64(s.vtxoTreeExpiry.Value))

	if err := s.sweeper.schedule(expirationTimestamp, round.Txid, round.VtxoTree); err != nil {
		log.WithError(err).Warn("failed to schedule sweep tx")
	}
}

func (s *covenantlessService) getNewVtxos(round *domain.Round) []domain.Vtxo {
	if len(round.VtxoTree) <= 0 {
		return nil
	}

	createdAt := time.Now().Unix()

	leaves := round.VtxoTree.Leaves()
	vtxos := make([]domain.Vtxo, 0)
	for _, node := range leaves {
		tx, err := psbt.NewFromRawBytes(strings.NewReader(node.Tx), true)
		if err != nil {
			log.WithError(err).Warn("failed to parse tx")
			continue
		}
		for i, out := range tx.UnsignedTx.TxOut {
			vtxoTapKey, err := schnorr.ParsePubKey(out.PkScript[2:])
			if err != nil {
				log.WithError(err).Warn("failed to parse vtxo tap key")
				continue
			}

			vtxoPubkey := hex.EncodeToString(schnorr.SerializePubKey(vtxoTapKey))
			vtxos = append(vtxos, domain.Vtxo{
				VtxoKey:   domain.VtxoKey{Txid: node.Txid, VOut: uint32(i)},
				PubKey:    vtxoPubkey,
				Amount:    uint64(out.Value),
				RoundTxid: round.Txid,
				CreatedAt: createdAt,
			})
		}
	}
	return vtxos
}

func (s *covenantlessService) getSpentVtxos(requests map[string]domain.TxRequest) []domain.Vtxo {
	outpoints := getSpentVtxos(requests)
	vtxos, _ := s.repoManager.Vtxos().GetVtxos(context.Background(), outpoints)
	return vtxos
}

func (s *covenantlessService) startWatchingVtxos(vtxos []domain.Vtxo) error {
	scripts, err := s.extractVtxosScripts(vtxos)
	if err != nil {
		return err
	}

	return s.scanner.WatchScripts(context.Background(), scripts)
}

func (s *covenantlessService) stopWatchingVtxos(vtxos []domain.Vtxo) {
	scripts, err := s.extractVtxosScripts(vtxos)
	if err != nil {
		log.WithError(err).Warn("failed to extract scripts from vtxos")
		return
	}

	for {
		if err := s.scanner.UnwatchScripts(context.Background(), scripts); err != nil {
			log.WithError(err).Warn("failed to stop watching vtxos, retrying in a moment...")
			time.Sleep(100 * time.Millisecond)
			continue
		}
		log.Debugf("stopped watching %d vtxos", len(vtxos))
		break
	}
}

func (s *covenantlessService) restoreWatchingVtxos() error {
	ctx := context.Background()

	expiredRounds, err := s.repoManager.Rounds().GetExpiredRoundsTxid(ctx)
	if err != nil {
		return err
	}

	vtxos := make([]domain.Vtxo, 0)
	for _, txid := range expiredRounds {
		fromRound, err := s.repoManager.Vtxos().GetVtxosForRound(ctx, txid)
		if err != nil {
			log.WithError(err).Warnf("failed to retrieve vtxos for round %s", txid)
			continue
		}
		for _, v := range fromRound {
			if !v.Swept && !v.Redeemed {
				vtxos = append(vtxos, v)
			}
		}
	}

	if len(vtxos) <= 0 {
		return nil
	}

	if err := s.startWatchingVtxos(vtxos); err != nil {
		return err
	}

	log.Debugf("restored watching %d vtxos", len(vtxos))
	return nil
}

func (s *covenantlessService) extractVtxosScripts(vtxos []domain.Vtxo) ([]string, error) {
	indexedScripts := make(map[string]struct{})

	for _, vtxo := range vtxos {
		vtxoTapKeyBytes, err := hex.DecodeString(vtxo.PubKey)
		if err != nil {
			return nil, err
		}

		vtxoTapKey, err := schnorr.ParsePubKey(vtxoTapKeyBytes)
		if err != nil {
			return nil, err
		}

		script, err := common.P2TRScript(vtxoTapKey)
		if err != nil {
			return nil, err
		}

		indexedScripts[hex.EncodeToString(script)] = struct{}{}
	}
	scripts := make([]string, 0, len(indexedScripts))
	for script := range indexedScripts {
		scripts = append(scripts, script)
	}
	return scripts, nil
}

func (s *covenantlessService) saveEvents(
	ctx context.Context, id string, events []domain.RoundEvent,
) error {
	if len(events) <= 0 {
		return nil
	}
	round, err := s.repoManager.Events().Save(ctx, id, events...)
	if err != nil {
		return err
	}
	return s.repoManager.Rounds().AddOrUpdateRound(ctx, *round)
}

func (s *covenantlessService) chainParams() *chaincfg.Params {
	switch s.network.Name {
	case common.Bitcoin.Name:
		return &chaincfg.MainNetParams
	case common.BitcoinTestNet.Name:
		return &chaincfg.TestNet3Params
	case common.BitcoinRegTest.Name:
		return &chaincfg.RegressionNetParams
	default:
		return nil
	}
}

func (s *covenantlessService) reactToFraud(ctx context.Context, vtxo domain.Vtxo, mutx *sync.Mutex) error {
	mutx.Lock()
	defer mutx.Unlock()
	roundRepo := s.repoManager.Rounds()

	round, err := roundRepo.GetRoundWithTxid(ctx, vtxo.SpentBy)
	if err != nil {
		vtxosRepo := s.repoManager.Vtxos()

		// If the round is not found, the utxo may be spent by an out of round tx
		vtxos, err := vtxosRepo.GetVtxos(ctx, []domain.VtxoKey{
			{Txid: vtxo.SpentBy, VOut: 0},
		})
		if err != nil || len(vtxos) <= 0 {
			return fmt.Errorf("failed to retrieve round: %s", err)
		}

		storedVtxo := vtxos[0]
		if storedVtxo.Redeemed { // redeem tx is already onchain
			return nil
		}

		log.Debugf("vtxo %s:%d has been spent by out of round transaction", vtxo.Txid, vtxo.VOut)

		redeemTxHex, err := s.builder.FinalizeAndExtract(storedVtxo.RedeemTx)
		if err != nil {
			return fmt.Errorf("failed to finalize redeem tx: %s", err)
		}

		redeemTxid, err := s.wallet.BroadcastTransaction(ctx, redeemTxHex)
		if err != nil {
			return fmt.Errorf("failed to broadcast redeem tx: %s", err)
		}

		log.Debugf("broadcasted redeem tx %s", redeemTxid)
		return nil
	}

	// Find the forfeit tx of the VTXO
	forfeitTx, err := findForfeitTxBitcoin(round.ForfeitTxs, vtxo.VtxoKey)
	if err != nil {
		return fmt.Errorf("failed to find forfeit tx: %s", err)
	}

	if len(forfeitTx.UnsignedTx.TxIn) <= 0 {
		return fmt.Errorf("invalid forfeit tx: %s", forfeitTx.UnsignedTx.TxHash().String())
	}

	connector := forfeitTx.UnsignedTx.TxIn[0]
	connectorOutpoint := txOutpoint{
		connector.PreviousOutPoint.Hash.String(),
		connector.PreviousOutPoint.Index,
	}

	// compute, sign and broadcast the branch txs until the connector outpoint is created
	branch, err := round.Connectors.Branch(connectorOutpoint.txid)
	if err != nil {
		return fmt.Errorf("failed to get branch of connector: %s", err)
	}

	for _, node := range branch {
		_, err := s.wallet.GetTransaction(ctx, node.Txid)
		// if err, it means the tx is offchain
		if err != nil {
			signedTx, err := s.wallet.SignTransaction(ctx, node.Tx, true)
			if err != nil {
				return fmt.Errorf("failed to sign tx: %s", err)
			}

			txid, err := s.wallet.BroadcastTransaction(ctx, signedTx)
			if err != nil {
				return fmt.Errorf("failed to broadcast transaction: %s", err)
			}
			log.Debugf("broadcasted transaction %s", txid)
		}
	}

	if err := s.wallet.LockConnectorUtxos(ctx, []ports.TxOutpoint{connectorOutpoint}); err != nil {
		return fmt.Errorf("failed to lock connector utxos: %s", err)
	}

	forfeitTxB64, err := forfeitTx.B64Encode()
	if err != nil {
		return fmt.Errorf("failed to encode forfeit tx: %s", err)
	}

	signedForfeitTx, err := s.wallet.SignTransactionTapscript(ctx, forfeitTxB64, nil)
	if err != nil {
		return fmt.Errorf("failed to sign forfeit tx: %s", err)
	}

	forfeitTxHex, err := s.builder.FinalizeAndExtract(signedForfeitTx)
	if err != nil {
		return fmt.Errorf("failed to finalize forfeit tx: %s", err)
	}

	forfeitTxid, err := s.wallet.BroadcastTransaction(ctx, forfeitTxHex)
	if err != nil {
		return fmt.Errorf("failed to broadcast forfeit tx: %s", err)
	}

	log.Debugf("broadcasted forfeit tx %s", forfeitTxid)
	return nil
}

func (s *covenantlessService) markAsRedeemed(ctx context.Context, vtxo domain.Vtxo) error {
	if err := s.repoManager.Vtxos().RedeemVtxos(ctx, []domain.VtxoKey{vtxo.VtxoKey}); err != nil {
		return err
	}

	log.Debugf("vtxo %s:%d redeemed", vtxo.Txid, vtxo.VOut)
	return nil
}

func (s *covenantlessService) verifyForfeitTxsSigs(txs []string) error {
	nbWorkers := runtime.NumCPU()
	jobs := make(chan string, len(txs))
	errChan := make(chan error, 1)
	wg := sync.WaitGroup{}
	wg.Add(nbWorkers)

	for i := 0; i < nbWorkers; i++ {
		go func() {
			defer wg.Done()

			for tx := range jobs {
				valid, txid, err := s.builder.VerifyTapscriptPartialSigs(tx)
				if err != nil {
					errChan <- fmt.Errorf("failed to validate forfeit tx %s: %s", txid, err)
					return
				}

				if !valid {
					errChan <- fmt.Errorf("invalid signature for forfeit tx %s", txid)
					return
				}
			}
		}()
	}

	for _, tx := range txs {
		select {
		case err := <-errChan:
			return err
		default:
			jobs <- tx
		}
	}
	close(jobs)
	wg.Wait()

	select {
	case err := <-errChan:
		return err
	default:
		close(errChan)
		return nil
	}
}

func findForfeitTxBitcoin(
	forfeits []string, vtxo domain.VtxoKey,
) (*psbt.Packet, error) {
	for _, forfeit := range forfeits {
		forfeitTx, err := psbt.NewFromRawBytes(strings.NewReader(forfeit), true)
		if err != nil {
			return nil, err
		}

		vtxoInput := forfeitTx.UnsignedTx.TxIn[1]

		if vtxoInput.PreviousOutPoint.Hash.String() == vtxo.Txid &&
			vtxoInput.PreviousOutPoint.Index == vtxo.VOut {
			return forfeitTx, nil
		}
	}

	return nil, fmt.Errorf("forfeit tx not found")
}

// musigSigningSession holds the state of ephemeral nonces and signatures in order to coordinate the signing of the tree
type musigSigningSession struct {
	lock        sync.Mutex
	nbCosigners int
	cosigners   map[string]struct{}
	nonces      map[*secp256k1.PublicKey]bitcointree.TreeNonces
	nonceDoneC  chan struct{}

	signatures map[*secp256k1.PublicKey]bitcointree.TreePartialSigs
	sigDoneC   chan struct{}
}

func newMusigSigningSession(cosigners map[string]struct{}) *musigSigningSession {
	return &musigSigningSession{
		nonces:     make(map[*secp256k1.PublicKey]bitcointree.TreeNonces),
		nonceDoneC: make(chan struct{}),

		signatures:  make(map[*secp256k1.PublicKey]bitcointree.TreePartialSigs),
		sigDoneC:    make(chan struct{}),
		lock:        sync.Mutex{},
		cosigners:   cosigners,
		nbCosigners: len(cosigners) + 1, // the server
	}
}

func (s *covenantlessService) GetMarketHourConfig(ctx context.Context) (*domain.MarketHour, error) {
	return s.repoManager.MarketHourRepo().Get(ctx)
}

func (s *covenantlessService) UpdateMarketHourConfig(
	ctx context.Context,
	marketHourStartTime, marketHourEndTime time.Time, period, roundInterval time.Duration,
) error {
	marketHour := domain.NewMarketHour(
		marketHourStartTime,
		marketHourEndTime,
		period,
		roundInterval,
	)
	if err := s.repoManager.MarketHourRepo().Upsert(ctx, *marketHour); err != nil {
		return fmt.Errorf("failed to upsert market hours: %w", err)
	}

	return nil
}

// Hashrate derivatives services

// BtcWalletAdapter adapts the WalletService to BtcWalletService interface
type btcWalletAdapter struct {
	wallet ports.WalletService
}

// GetBlockInfo implements BtcWalletService.GetBlockInfo by converting ports.BlockInfo to application.BlockInfo
func (a *btcWalletAdapter) GetBlockInfo(height int32) (*BlockInfo, error) {
	portBlockInfo, err := a.wallet.GetBlockInfo(height)
	if err != nil {
		return nil, err
	}
	return &BlockInfo{
		Height:     portBlockInfo.Height,
		Hash:       portBlockInfo.Hash,
		Timestamp:  portBlockInfo.Timestamp,
		Difficulty: portBlockInfo.Difficulty,
	}, nil
}

// GetCurrentHeight implements BtcWalletService.GetCurrentHeight
func (a *btcWalletAdapter) GetCurrentHeight() (int32, error) {
	return a.wallet.GetCurrentHeight()
}

// ContractService returns the contract service for hashrate derivatives
func (s *covenantlessService) ContractService() *ContractService {
	// Initialize the contract service if needed
	eventPublisher := NewEventPublisher()
	
	// Create adapter to convert between WalletService and BtcWalletService
	walletAdapter := &btcWalletAdapter{wallet: s.wallet}
	
	return NewContractService(
		s.repoManager,
		walletAdapter,
		s.repoManager.SchedulerService(),
		eventPublisher,
	)
}

// OrderBookService returns the orderbook service for hashrate derivatives
func (s *covenantlessService) OrderBookService() *OrderBookService {
	// Initialize the orderbook service if needed
	eventPublisher := NewEventPublisher()
	
	return NewOrderBookService(
		s.repoManager,
		s.ContractService(),
		eventPublisher,
	)
}

// HashrateCalculator returns the hashrate calculator for Bitcoin network
func (s *covenantlessService) HashrateCalculator() *HashrateCalculator {
	// Create adapter to convert between WalletService and BtcWalletService
	walletAdapter := &btcWalletAdapter{wallet: s.wallet}
	return NewHashrateCalculator(walletAdapter)
}

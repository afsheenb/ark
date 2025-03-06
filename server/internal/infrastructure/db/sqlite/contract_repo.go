package sqlitedb

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/ark-network/ark/server/internal/core/domain"
)

// ContractRepository is a SQLite implementation of domain.ContractRepository
type ContractRepository struct {
	db *sql.DB
}

// NewContractRepository creates a new SQLite contract repository
func NewContractRepository(db *sql.DB) *ContractRepository {
	return &ContractRepository{
		db: db,
	}
}

// Save stores a hashrate contract
func (r *ContractRepository) Save(contract *domain.HashrateContract) error {
	// First check if the contract exists
	var exists bool
	err := r.db.QueryRow("SELECT 1 FROM hashrate_contracts WHERE id = ?", contract.ID).Scan(&exists)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to check if contract exists: %w", err)
	}

	// If contract exists, update it
	if err == nil {
		_, err = r.db.Exec(`
			UPDATE hashrate_contracts
			SET contract_type = ?, strike_hash_rate = ?, start_block_height = ?, 
				end_block_height = ?, target_timestamp = ?, contract_size = ?, 
				premium = ?, buyer_pubkey = ?, seller_pubkey = ?, status = ?, 
				updated_at = ?, expires_at = ?, setup_txid = ?, final_txid = ?, 
				settlement_txid = ?
			WHERE id = ?
		`,
			string(contract.ContractType),
			contract.StrikeHashRate,
			contract.StartBlockHeight,
			contract.EndBlockHeight,
			contract.TargetTimestamp,
			contract.ContractSize,
			contract.Premium,
			contract.BuyerPubkey,
			contract.SellerPubkey,
			string(contract.Status),
			contract.UpdatedAt,
			contract.ExpiresAt,
			contract.SetupTxid,
			contract.FinalTxid,
			contract.SettlementTxid,
			contract.ID,
		)
		if err != nil {
			return fmt.Errorf("failed to update contract: %w", err)
		}
		return nil
	}

	// Otherwise, insert a new contract
	_, err = r.db.Exec(`
		INSERT INTO hashrate_contracts (
			id, contract_type, strike_hash_rate, start_block_height, 
			end_block_height, target_timestamp, contract_size, premium, 
			buyer_pubkey, seller_pubkey, status, created_at, updated_at, 
			expires_at, setup_txid, final_txid, settlement_txid
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		contract.ID,
		string(contract.ContractType),
		contract.StrikeHashRate,
		contract.StartBlockHeight,
		contract.EndBlockHeight,
		contract.TargetTimestamp,
		contract.ContractSize,
		contract.Premium,
		contract.BuyerPubkey,
		contract.SellerPubkey,
		string(contract.Status),
		contract.CreatedAt,
		contract.UpdatedAt,
		contract.ExpiresAt,
		contract.SetupTxid,
		contract.FinalTxid,
		contract.SettlementTxid,
	)
	if err != nil {
		return fmt.Errorf("failed to insert contract: %w", err)
	}
	return nil
}

// FindByID retrieves a hashrate contract by ID
func (r *ContractRepository) FindByID(id string) (*domain.HashrateContract, error) {
	row := r.db.QueryRow(`
		SELECT id, contract_type, strike_hash_rate, start_block_height, 
			end_block_height, target_timestamp, contract_size, premium, 
			buyer_pubkey, seller_pubkey, status, created_at, updated_at, 
			expires_at, setup_txid, final_txid, settlement_txid
		FROM hashrate_contracts
		WHERE id = ?
	`, id)

	return r.scanContract(row)
}

// FindByStatus retrieves hashrate contracts by status
func (r *ContractRepository) FindByStatus(status domain.ContractStatus) ([]*domain.HashrateContract, error) {
	rows, err := r.db.Query(`
		SELECT id, contract_type, strike_hash_rate, start_block_height, 
			end_block_height, target_timestamp, contract_size, premium, 
			buyer_pubkey, seller_pubkey, status, created_at, updated_at, 
			expires_at, setup_txid, final_txid, settlement_txid
		FROM hashrate_contracts
		WHERE status = ?
		ORDER BY created_at DESC
	`, string(status))
	if err != nil {
		return nil, fmt.Errorf("failed to query contracts by status: %w", err)
	}
	defer rows.Close()

	return r.scanContracts(rows)
}

// FindByUser retrieves hashrate contracts where user is buyer or seller
func (r *ContractRepository) FindByUser(pubkey string) ([]*domain.HashrateContract, error) {
	rows, err := r.db.Query(`
		SELECT id, contract_type, strike_hash_rate, start_block_height, 
			end_block_height, target_timestamp, contract_size, premium, 
			buyer_pubkey, seller_pubkey, status, created_at, updated_at, 
			expires_at, setup_txid, final_txid, settlement_txid
		FROM hashrate_contracts
		WHERE buyer_pubkey = ? OR seller_pubkey = ?
		ORDER BY created_at DESC
	`, pubkey, pubkey)
	if err != nil {
		return nil, fmt.Errorf("failed to query contracts by user: %w", err)
	}
	defer rows.Close()

	return r.scanContracts(rows)
}

// FindAll retrieves all hashrate contracts with optional pagination
func (r *ContractRepository) FindAll(limit, offset int) ([]*domain.HashrateContract, error) {
	query := `
		SELECT id, contract_type, strike_hash_rate, start_block_height, 
			end_block_height, target_timestamp, contract_size, premium, 
			buyer_pubkey, seller_pubkey, status, created_at, updated_at, 
			expires_at, setup_txid, final_txid, settlement_txid
		FROM hashrate_contracts
		ORDER BY created_at DESC
	`

	// Add pagination if needed
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
	}

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query all contracts: %w", err)
	}
	defer rows.Close()

	return r.scanContracts(rows)
}

// CountAll counts all hashrate contracts
func (r *ContractRepository) CountAll() (int, error) {
	var count int
	err := r.db.QueryRow("SELECT COUNT(*) FROM hashrate_contracts").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count contracts: %w", err)
	}
	return count, nil
}

// scanContract scans a single contract from a row
func (r *ContractRepository) scanContract(row *sql.Row) (*domain.HashrateContract, error) {
	var contract domain.HashrateContract
	var contractTypeStr, statusStr string
	var setupTxid, finalTxid, settlementTxid sql.NullString
	var expiresAt sql.NullInt64

	err := row.Scan(
		&contract.ID,
		&contractTypeStr,
		&contract.StrikeHashRate,
		&contract.StartBlockHeight,
		&contract.EndBlockHeight,
		&contract.TargetTimestamp,
		&contract.ContractSize,
		&contract.Premium,
		&contract.BuyerPubkey,
		&contract.SellerPubkey,
		&statusStr,
		&contract.CreatedAt,
		&contract.UpdatedAt,
		&expiresAt,
		&setupTxid,
		&finalTxid,
		&settlementTxid,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("contract not found")
		}
		return nil, fmt.Errorf("failed to scan contract: %w", err)
	}

	// Convert string types to domain types
	contract.ContractType = domain.ContractType(contractTypeStr)
	contract.Status = domain.ContractStatus(statusStr)

	// Handle nullable fields
	if expiresAt.Valid {
		contract.ExpiresAt = expiresAt.Int64
	}
	if setupTxid.Valid {
		contract.SetupTxid = setupTxid.String
	}
	if finalTxid.Valid {
		contract.FinalTxid = finalTxid.String
	}
	if settlementTxid.Valid {
		contract.SettlementTxid = settlementTxid.String
	}

	return &contract, nil
}

// scanContracts scans multiple contracts from rows
func (r *ContractRepository) scanContracts(rows *sql.Rows) ([]*domain.HashrateContract, error) {
	var contracts []*domain.HashrateContract

	for rows.Next() {
		var contract domain.HashrateContract
		var contractTypeStr, statusStr string
		var setupTxid, finalTxid, settlementTxid sql.NullString
		var expiresAt sql.NullInt64

		err := rows.Scan(
			&contract.ID,
			&contractTypeStr,
			&contract.StrikeHashRate,
			&contract.StartBlockHeight,
			&contract.EndBlockHeight,
			&contract.TargetTimestamp,
			&contract.ContractSize,
			&contract.Premium,
			&contract.BuyerPubkey,
			&contract.SellerPubkey,
			&statusStr,
			&contract.CreatedAt,
			&contract.UpdatedAt,
			&expiresAt,
			&setupTxid,
			&finalTxid,
			&settlementTxid,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan contract row: %w", err)
		}

		// Convert string types to domain types
		contract.ContractType = domain.ContractType(contractTypeStr)
		contract.Status = domain.ContractStatus(statusStr)

		// Handle nullable fields
		if expiresAt.Valid {
			contract.ExpiresAt = expiresAt.Int64
		}
		if setupTxid.Valid {
			contract.SetupTxid = setupTxid.String
		}
		if finalTxid.Valid {
			contract.FinalTxid = finalTxid.String
		}
		if settlementTxid.Valid {
			contract.SettlementTxid = settlementTxid.String
		}

		contracts = append(contracts, &contract)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating contract rows: %w", err)
	}

	return contracts, nil
}

// HashrateRepository is a SQLite implementation of domain.HashrateRepository
type HashrateRepository struct {
	db *sql.DB
}

// NewHashrateRepository creates a new SQLite hashrate repository
func NewHashrateRepository(db *sql.DB) *HashrateRepository {
	return &HashrateRepository{
		db: db,
	}
}

// SaveHashrate stores a hashrate measurement
func (r *HashrateRepository) SaveHashrate(data *domain.HashrateData) error {
	// First check if measurement exists
	var exists bool
	err := r.db.QueryRow("SELECT 1 FROM historical_hashrates WHERE block_height = ?", 
		data.BlockHeight).Scan(&exists)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to check if hashrate exists: %w", err)
	}

	// If it exists, update it
	if err == nil {
		_, err = r.db.Exec(`
			UPDATE historical_hashrates
			SET timestamp = ?, hashrate = ?, difficulty = ?
			WHERE block_height = ?
		`,
			data.Timestamp,
			data.Hashrate,
			data.Difficulty,
			data.BlockHeight,
		)
		if err != nil {
			return fmt.Errorf("failed to update hashrate: %w", err)
		}
		return nil
	}

	// Otherwise, insert a new record
	_, err = r.db.Exec(`
		INSERT INTO historical_hashrates (block_height, timestamp, hashrate, difficulty)
		VALUES (?, ?, ?, ?)
	`,
		data.BlockHeight,
		data.Timestamp,
		data.Hashrate,
		data.Difficulty,
	)
	if err != nil {
		return fmt.Errorf("failed to insert hashrate: %w", err)
	}
	return nil
}

// GetHashrateAt retrieves the hashrate at a specific block height
func (r *HashrateRepository) GetHashrateAt(blockHeight int32) (*domain.HashrateData, error) {
	row := r.db.QueryRow(`
		SELECT block_height, timestamp, hashrate, difficulty
		FROM historical_hashrates
		WHERE block_height = ?
	`, blockHeight)

	return r.scanHashrateData(row)
}

// GetHashrateRange retrieves hashrates between two block heights
func (r *HashrateRepository) GetHashrateRange(startHeight, endHeight int32) ([]*domain.HashrateData, error) {
	rows, err := r.db.Query(`
		SELECT block_height, timestamp, hashrate, difficulty
		FROM historical_hashrates
		WHERE block_height BETWEEN ? AND ?
		ORDER BY block_height ASC
	`, startHeight, endHeight)
	if err != nil {
		return nil, fmt.Errorf("failed to query hashrate range: %w", err)
	}
	defer rows.Close()

	return r.scanHashrateDataList(rows)
}

// GetLatestHashrate retrieves the most recent hashrate measurement
func (r *HashrateRepository) GetLatestHashrate() (*domain.HashrateData, error) {
	row := r.db.QueryRow(`
		SELECT block_height, timestamp, hashrate, difficulty
		FROM historical_hashrates
		ORDER BY block_height DESC
		LIMIT 1
	`)

	return r.scanHashrateData(row)
}

// scanHashrateData scans a single hashrate data from a row
func (r *HashrateRepository) scanHashrateData(row *sql.Row) (*domain.HashrateData, error) {
	var data domain.HashrateData
	err := row.Scan(
		&data.BlockHeight,
		&data.Timestamp,
		&data.Hashrate,
		&data.Difficulty,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("hashrate data not found")
		}
		return nil, fmt.Errorf("failed to scan hashrate data: %w", err)
	}
	return &data, nil
}

// scanHashrateDataList scans multiple hashrate data from rows
func (r *HashrateRepository) scanHashrateDataList(rows *sql.Rows) ([]*domain.HashrateData, error) {
	var dataList []*domain.HashrateData

	for rows.Next() {
		var data domain.HashrateData
		err := rows.Scan(
			&data.BlockHeight,
			&data.Timestamp,
			&data.Hashrate,
			&data.Difficulty,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan hashrate data row: %w", err)
		}
		dataList = append(dataList, &data)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating hashrate data rows: %w", err)
	}

	return dataList, nil
}

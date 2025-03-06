package sqlitedb

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/ark-network/ark/server/internal/core/domain"
)

// OrderRepository is a SQLite implementation of domain.OrderRepository
type OrderRepository struct {
	db *sql.DB
}

// NewOrderRepository creates a new SQLite order repository
func NewOrderRepository(db *sql.DB) *OrderRepository {
	return &OrderRepository{
		db: db,
	}
}

// Save stores an order
func (r *OrderRepository) Save(order *domain.Order) error {
	// First check if the order exists
	var exists bool
	err := r.db.QueryRow("SELECT 1 FROM orders WHERE id = ?", order.ID).Scan(&exists)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to check if order exists: %w", err)
	}

	// If order exists, update it
	if err == nil {
		_, err = r.db.Exec(`
			UPDATE orders
			SET side = ?, contract_type = ?, strike_hash_rate = ?, 
				start_block_height = ?, end_block_height = ?, price = ?, 
				quantity = ?, remaining_quantity = ?, status = ?, 
				pubkey = ?, updated_at = ?, expires_at = ?
			WHERE id = ?
		`,
			string(order.Side),
			string(order.ContractType),
			order.StrikeHashRate,
			order.StartBlockHeight,
			order.EndBlockHeight,
			order.Price,
			order.Quantity,
			order.RemainingQuantity,
			string(order.Status),
			order.Pubkey,
			order.UpdatedAt,
			order.ExpiresAt,
			order.ID,
		)
		if err != nil {
			return fmt.Errorf("failed to update order: %w", err)
		}
		return nil
	}

	// Otherwise, insert a new order
	_, err = r.db.Exec(`
		INSERT INTO orders (
			id, user_id, side, contract_type, strike_hash_rate, 
			start_block_height, end_block_height, price, quantity, 
			remaining_quantity, status, pubkey, created_at, updated_at, 
			expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		order.ID,
		order.UserID,
		string(order.Side),
		string(order.ContractType),
		order.StrikeHashRate,
		order.StartBlockHeight,
		order.EndBlockHeight,
		order.Price,
		order.Quantity,
		order.RemainingQuantity,
		string(order.Status),
		order.Pubkey,
		order.CreatedAt,
		order.UpdatedAt,
		order.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert order: %w", err)
	}
	return nil
}

// FindByID retrieves an order by ID
func (r *OrderRepository) FindByID(id string) (*domain.Order, error) {
	row := r.db.QueryRow(`
		SELECT id, user_id, side, contract_type, strike_hash_rate, 
			start_block_height, end_block_height, price, quantity, 
			remaining_quantity, status, pubkey, created_at, updated_at, 
			expires_at
		FROM orders
		WHERE id = ?
	`, id)

	return r.scanOrder(row)
}

// FindOpenOrders retrieves open orders matching contract specifications
func (r *OrderRepository) FindOpenOrders(
	contractType domain.ContractType,
	strikeHashRate float64,
	startBlockHeight, endBlockHeight int32,
) ([]*domain.Order, error) {
	rows, err := r.db.Query(`
		SELECT id, user_id, side, contract_type, strike_hash_rate, 
			start_block_height, end_block_height, price, quantity, 
			remaining_quantity, status, pubkey, created_at, updated_at, 
			expires_at
		FROM orders
		WHERE contract_type = ? AND strike_hash_rate = ? AND 
			start_block_height = ? AND end_block_height = ? AND
			(status = ? OR status = ?)
		ORDER BY created_at ASC
	`,
		string(contractType),
		strikeHashRate,
		startBlockHeight,
		endBlockHeight,
		string(domain.OrderStatusOpen),
		string(domain.OrderStatusPartial),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query open orders: %w", err)
	}
	defer rows.Close()

	return r.scanOrders(rows)
}

// FindByUser retrieves orders for a specific user
func (r *OrderRepository) FindByUser(
	userID string,
	status domain.OrderStatus,
	limit, offset int,
) ([]*domain.Order, error) {
	query := `
		SELECT id, user_id, side, contract_type, strike_hash_rate, 
			start_block_height, end_block_height, price, quantity, 
			remaining_quantity, status, pubkey, created_at, updated_at, 
			expires_at
		FROM orders
		WHERE user_id = ?
	`
	args := []interface{}{userID}

	// Add status filter if specified
	if status != "" {
		query += " AND status = ?"
		args = append(args, string(status))
	}

	query += " ORDER BY created_at DESC"

	// Add pagination if needed
	if limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, limit, offset)
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query user orders: %w", err)
	}
	defer rows.Close()

	return r.scanOrders(rows)
}

// CountByUser counts orders for a specific user
func (r *OrderRepository) CountByUser(userID string, status domain.OrderStatus) (int, error) {
	query := "SELECT COUNT(*) FROM orders WHERE user_id = ?"
	args := []interface{}{userID}

	// Add status filter if specified
	if status != "" {
		query += " AND status = ?"
		args = append(args, string(status))
	}

	var count int
	err := r.db.QueryRow(query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count user orders: %w", err)
	}
	return count, nil
}

// FindAll retrieves all orders with optional pagination
func (r *OrderRepository) FindAll(limit, offset int) ([]*domain.Order, error) {
	query := `
		SELECT id, user_id, side, contract_type, strike_hash_rate, 
			start_block_height, end_block_height, price, quantity, 
			remaining_quantity, status, pubkey, created_at, updated_at, 
			expires_at
		FROM orders
		ORDER BY created_at DESC
	`

	// Add pagination if needed
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
	}

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query all orders: %w", err)
	}
	defer rows.Close()

	return r.scanOrders(rows)
}

// CountAll counts all orders
func (r *OrderRepository) CountAll() (int, error) {
	var count int
	err := r.db.QueryRow("SELECT COUNT(*) FROM orders").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count orders: %w", err)
	}
	return count, nil
}

// scanOrder scans a single order from a row
func (r *OrderRepository) scanOrder(row *sql.Row) (*domain.Order, error) {
	var order domain.Order
	var sideStr, contractTypeStr, statusStr string
	var expiresAt sql.NullInt64

	err := row.Scan(
		&order.ID,
		&order.UserID,
		&sideStr,
		&contractTypeStr,
		&order.StrikeHashRate,
		&order.StartBlockHeight,
		&order.EndBlockHeight,
		&order.Price,
		&order.Quantity,
		&order.RemainingQuantity,
		&statusStr,
		&order.Pubkey,
		&order.CreatedAt,
		&order.UpdatedAt,
		&expiresAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("order not found")
		}
		return nil, fmt.Errorf("failed to scan order: %w", err)
	}

	// Convert string types to domain types
	order.Side = domain.OrderSide(sideStr)
	order.ContractType = domain.ContractType(contractTypeStr)
	order.Status = domain.OrderStatus(statusStr)

	// Handle nullable fields
	if expiresAt.Valid {
		order.ExpiresAt = expiresAt.Int64
	}

	return &order, nil
}

// scanOrders scans multiple orders from rows
func (r *OrderRepository) scanOrders(rows *sql.Rows) ([]*domain.Order, error) {
	var orders []*domain.Order

	for rows.Next() {
		var order domain.Order
		var sideStr, contractTypeStr, statusStr string
		var expiresAt sql.NullInt64

		err := rows.Scan(
			&order.ID,
			&order.UserID,
			&sideStr,
			&contractTypeStr,
			&order.StrikeHashRate,
			&order.StartBlockHeight,
			&order.EndBlockHeight,
			&order.Price,
			&order.Quantity,
			&order.RemainingQuantity,
			&statusStr,
			&order.Pubkey,
			&order.CreatedAt,
			&order.UpdatedAt,
			&expiresAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan order row: %w", err)
		}

		// Convert string types to domain types
		order.Side = domain.OrderSide(sideStr)
		order.ContractType = domain.ContractType(contractTypeStr)
		order.Status = domain.OrderStatus(statusStr)

		// Handle nullable fields
		if expiresAt.Valid {
			order.ExpiresAt = expiresAt.Int64
		}

		orders = append(orders, &order)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating order rows: %w", err)
	}

	return orders, nil
}

// TradeRepository is a SQLite implementation of domain.TradeRepository
type TradeRepository struct {
	db *sql.DB
}

// NewTradeRepository creates a new SQLite trade repository
func NewTradeRepository(db *sql.DB) *TradeRepository {
	return &TradeRepository{
		db: db,
	}
}

// Save stores a trade
func (r *TradeRepository) Save(trade *domain.Trade) error {
	// First check if the trade exists
	var exists bool
	err := r.db.QueryRow("SELECT 1 FROM trades WHERE id = ?", trade.ID).Scan(&exists)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to check if trade exists: %w", err)
	}

	// If trade exists, just return (trades are immutable)
	if err == nil {
		return nil
	}

	// Otherwise, insert a new trade
	_, err = r.db.Exec(`
		INSERT INTO trades (id, buy_order_id, sell_order_id, price, quantity, executed_at, contract_id)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		trade.ID,
		trade.BuyOrderID,
		trade.SellOrderID,
		trade.Price,
		trade.Quantity,
		trade.ExecutedAt,
		trade.ContractID,
	)
	if err != nil {
		return fmt.Errorf("failed to insert trade: %w", err)
	}
	return nil
}

// FindByID retrieves a trade by ID
func (r *TradeRepository) FindByID(id string) (*domain.Trade, error) {
	row := r.db.QueryRow(`
		SELECT id, buy_order_id, sell_order_id, price, quantity, executed_at, contract_id
		FROM trades
		WHERE id = ?
	`, id)

	return r.scanTrade(row)
}

// FindByContract retrieves trades for a specific contract specification
func (r *TradeRepository) FindByContract(
	contractType domain.ContractType,
	strikeHashRate float64,
	startBlockHeight, endBlockHeight int32,
	limit, offset int,
) ([]*domain.Trade, error) {
	// Get buy order IDs matching the contract specification
	buyOrderRows, err := r.db.Query(`
		SELECT id FROM orders
		WHERE contract_type = ? AND strike_hash_rate = ? AND 
			start_block_height = ? AND end_block_height = ? AND
			side = ?
	`,
		string(contractType),
		strikeHashRate,
		startBlockHeight,
		endBlockHeight,
		string(domain.OrderSideBuy),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query buy orders: %w", err)
	}
	defer buyOrderRows.Close()

	// Extract buy order IDs
	var buyOrderIDs []string
	for buyOrderRows.Next() {
		var id string
		if err := buyOrderRows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan buy order ID: %w", err)
		}
		buyOrderIDs = append(buyOrderIDs, id)
	}
	if err := buyOrderRows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating buy order rows: %w", err)
	}

	// If no matching buy orders, return empty list
	if len(buyOrderIDs) == 0 {
		return []*domain.Trade{}, nil
	}

	// Create the SQL query with a placeholder for each buy order ID
	placeholders := strings.Repeat("?,", len(buyOrderIDs))
	placeholders = placeholders[:len(placeholders)-1] // Remove trailing comma

	query := fmt.Sprintf(`
		SELECT id, buy_order_id, sell_order_id, price, quantity, executed_at, contract_id
		FROM trades
		WHERE buy_order_id IN (%s)
		ORDER BY executed_at DESC
	`, placeholders)

	// Add pagination if needed
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
	}

	// Convert buyOrderIDs to []interface{} for the query
	args := make([]interface{}, len(buyOrderIDs))
	for i, id := range buyOrderIDs {
		args[i] = id
	}

	// Execute the query
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query trades by contract: %w", err)
	}
	defer rows.Close()

	return r.scanTrades(rows)
}

// CountByContract counts trades for a specific contract specification
func (r *TradeRepository) CountByContract(
	contractType domain.ContractType,
	strikeHashRate float64,
	startBlockHeight, endBlockHeight int32,
) (int, error) {
	// Get buy order IDs matching the contract specification
	buyOrderRows, err := r.db.Query(`
		SELECT id FROM orders
		WHERE contract_type = ? AND strike_hash_rate = ? AND 
			start_block_height = ? AND end_block_height = ? AND
			side = ?
	`,
		string(contractType),
		strikeHashRate,
		startBlockHeight,
		endBlockHeight,
		string(domain.OrderSideBuy),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to query buy orders: %w", err)
	}
	defer buyOrderRows.Close()

	// Extract buy order IDs
	var buyOrderIDs []string
	for buyOrderRows.Next() {
		var id string
		if err := buyOrderRows.Scan(&id); err != nil {
			return 0, fmt.Errorf("failed to scan buy order ID: %w", err)
		}
		buyOrderIDs = append(buyOrderIDs, id)
	}
	if err := buyOrderRows.Err(); err != nil {
		return 0, fmt.Errorf("error iterating buy order rows: %w", err)
	}

	// If no matching buy orders, return 0
	if len(buyOrderIDs) == 0 {
		return 0, nil
	}

	// Create the SQL query with a placeholder for each buy order ID
	placeholders := strings.Repeat("?,", len(buyOrderIDs))
	placeholders = placeholders[:len(placeholders)-1] // Remove trailing comma

	query := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM trades
		WHERE buy_order_id IN (%s)
	`, placeholders)

	// Convert buyOrderIDs to []interface{} for the query
	args := make([]interface{}, len(buyOrderIDs))
	for i, id := range buyOrderIDs {
		args[i] = id
	}

	// Execute the query
	var count int
	err = r.db.QueryRow(query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count trades by contract: %w", err)
	}

	return count, nil
}

// FindByOrder retrieves trades involving a specific order
func (r *TradeRepository) FindByOrder(orderID string) ([]*domain.Trade, error) {
	rows, err := r.db.Query(`
		SELECT id, buy_order_id, sell_order_id, price, quantity, executed_at, contract_id
		FROM trades
		WHERE buy_order_id = ? OR sell_order_id = ?
		ORDER BY executed_at DESC
	`, orderID, orderID)
	if err != nil {
		return nil, fmt.Errorf("failed to query trades by order: %w", err)
	}
	defer rows.Close()

	return r.scanTrades(rows)
}

// FindAll retrieves all trades with optional pagination
func (r *TradeRepository) FindAll(limit, offset int) ([]*domain.Trade, error) {
	query := `
		SELECT id, buy_order_id, sell_order_id, price, quantity, executed_at, contract_id
		FROM trades
		ORDER BY executed_at DESC
	`

	// Add pagination if needed
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
	}

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query all trades: %w", err)
	}
	defer rows.Close()

	return r.scanTrades(rows)
}

// CountAll counts all trades
func (r *TradeRepository) CountAll() (int, error) {
	var count int
	err := r.db.QueryRow("SELECT COUNT(*) FROM trades").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count trades: %w", err)
	}
	return count, nil
}

// scanTrade scans a single trade from a row
func (r *TradeRepository) scanTrade(row *sql.Row) (*domain.Trade, error) {
	var trade domain.Trade
	var contractID sql.NullString

	err := row.Scan(
		&trade.ID,
		&trade.BuyOrderID,
		&trade.SellOrderID,
		&trade.Price,
		&trade.Quantity,
		&trade.ExecutedAt,
		&contractID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("trade not found")
		}
		return nil, fmt.Errorf("failed to scan trade: %w", err)
	}

	// Handle nullable fields
	if contractID.Valid {
		trade.ContractID = contractID.String
	}

	return &trade, nil
}

// scanTrades scans multiple trades from rows
func (r *TradeRepository) scanTrades(rows *sql.Rows) ([]*domain.Trade, error) {
	var trades []*domain.Trade

	for rows.Next() {
		var trade domain.Trade
		var contractID sql.NullString

		err := rows.Scan(
			&trade.ID,
			&trade.BuyOrderID,
			&trade.SellOrderID,
			&trade.Price,
			&trade.Quantity,
			&trade.ExecutedAt,
			&contractID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan trade row: %w", err)
		}

		// Handle nullable fields
		if contractID.Valid {
			trade.ContractID = contractID.String
		}

		trades = append(trades, &trade)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating trade rows: %w", err)
	}

	return trades, nil
}

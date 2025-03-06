package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// OrderSide represents whether an order is buying or selling
type OrderSide string

const (
	// OrderSideBuy represents a buy (bid) order
	OrderSideBuy OrderSide = "BUY"
	// OrderSideSell represents a sell (ask) order
	OrderSideSell OrderSide = "SELL"
)

// OrderStatus represents the status of an order
type OrderStatus string

const (
	// OrderStatusOpen indicates an order is open and available for matching
	OrderStatusOpen OrderStatus = "OPEN"
	// OrderStatusPartial indicates an order has been partially filled
	OrderStatusPartial OrderStatus = "PARTIAL"
	// OrderStatusFilled indicates an order has been completely filled
	OrderStatusFilled OrderStatus = "FILLED"
	// OrderStatusCanceled indicates an order has been canceled
	OrderStatusCanceled OrderStatus = "CANCELED"
	// OrderStatusExpired indicates an order has expired
	OrderStatusExpired OrderStatus = "EXPIRED"
)

// Order represents an order in the orderbook
type Order struct {
	ID                string       `json:"id"`
	UserID            string       `json:"user_id"`
	Side              OrderSide    `json:"side"`
	ContractType      ContractType `json:"contract_type"`
	StrikeHashRate    float64      `json:"strike_hash_rate"`
	StartBlockHeight  int32        `json:"start_block_height"`
	EndBlockHeight    int32        `json:"end_block_height"`
	Price             uint64       `json:"price"` // in satoshis
	Quantity          int32        `json:"quantity"`
	RemainingQuantity int32        `json:"remaining_quantity"`
	Status            OrderStatus  `json:"status"`
	Pubkey            string       `json:"pubkey"`
	CreatedAt         int64        `json:"created_at"`
	UpdatedAt         int64        `json:"updated_at"`
	ExpiresAt         int64        `json:"expires_at,omitempty"`
}

// NewOrder creates a new order
func NewOrder(
	userID string,
	side OrderSide,
	contractType ContractType,
	strikeHashRate float64,
	startBlockHeight int32,
	endBlockHeight int32,
	price uint64,
	quantity int32,
	pubkey string,
	expiresAt int64,
) (*Order, error) {
	// Validate inputs
	if userID == "" {
		return nil, errors.New("user ID must be specified")
	}
	if side != OrderSideBuy && side != OrderSideSell {
		return nil, errors.New("side must be BUY or SELL")
	}
	if contractType != ContractTypeCall && contractType != ContractTypePut {
		return nil, errors.New("contract type must be CALL or PUT")
	}
	if strikeHashRate <= 0 {
		return nil, errors.New("strike hash rate must be positive")
	}
	if startBlockHeight >= endBlockHeight {
		return nil, errors.New("end block height must be greater than start block height")
	}
	if price == 0 {
		return nil, errors.New("price must be positive")
	}
	if quantity <= 0 {
		return nil, errors.New("quantity must be positive")
	}
	if pubkey == "" {
		return nil, errors.New("pubkey must be specified")
	}

	now := time.Now().Unix()

	return &Order{
		ID:                uuid.New().String(),
		UserID:            userID,
		Side:              side,
		ContractType:      contractType,
		StrikeHashRate:    strikeHashRate,
		StartBlockHeight:  startBlockHeight,
		EndBlockHeight:    endBlockHeight,
		Price:             price,
		Quantity:          quantity,
		RemainingQuantity: quantity,
		Status:            OrderStatusOpen,
		Pubkey:            pubkey,
		CreatedAt:         now,
		UpdatedAt:         now,
		ExpiresAt:         expiresAt,
	}, nil
}

// Fill updates the order when it is filled (partially or completely)
func (o *Order) Fill(filledQuantity int32) error {
	if filledQuantity <= 0 {
		return errors.New("filled quantity must be positive")
	}
	if filledQuantity > o.RemainingQuantity {
		return errors.New("filled quantity cannot exceed remaining quantity")
	}
	if o.Status == OrderStatusCanceled || o.Status == OrderStatusExpired {
		return errors.New("cannot fill a canceled or expired order")
	}

	o.RemainingQuantity -= filledQuantity
	if o.RemainingQuantity == 0 {
		o.Status = OrderStatusFilled
	} else {
		o.Status = OrderStatusPartial
	}
	o.UpdatedAt = time.Now().Unix()
	return nil
}

// Cancel cancels the order
func (o *Order) Cancel() error {
	if o.Status == OrderStatusFilled || o.Status == OrderStatusCanceled || o.Status == OrderStatusExpired {
		return errors.New("cannot cancel an order that is filled, canceled, or expired")
	}

	o.Status = OrderStatusCanceled
	o.UpdatedAt = time.Now().Unix()
	return nil
}

// Expire marks the order as expired
func (o *Order) Expire() error {
	if o.Status == OrderStatusFilled || o.Status == OrderStatusCanceled || o.Status == OrderStatusExpired {
		return errors.New("cannot expire an order that is filled, canceled, or expired")
	}

	o.Status = OrderStatusExpired
	o.UpdatedAt = time.Now().Unix()
	return nil
}

// IsExpired checks if the order has expired
func (o *Order) IsExpired(currentTime int64) bool {
	return o.ExpiresAt > 0 && currentTime > o.ExpiresAt
}

// CanMatch checks if this order can match with another order
func (o *Order) CanMatch(other *Order) bool {
	// Orders must be on opposite sides
	if o.Side == other.Side {
		return false
	}

	// Contract specifications must match exactly
	if o.ContractType != other.ContractType ||
		o.StrikeHashRate != other.StrikeHashRate ||
		o.StartBlockHeight != other.StartBlockHeight ||
		o.EndBlockHeight != other.EndBlockHeight {
		return false
	}

	// Price must match: for a match, buy price >= sell price
	var buyOrder, sellOrder *Order
	if o.Side == OrderSideBuy {
		buyOrder, sellOrder = o, other
	} else {
		buyOrder, sellOrder = other, o
	}
	
	return buyOrder.Price >= sellOrder.Price
}

// Trade represents a completed trade between two orders
type Trade struct {
	ID           string `json:"id"`
	BuyOrderID   string `json:"buy_order_id"`
	SellOrderID  string `json:"sell_order_id"`
	Price        uint64 `json:"price"` // in satoshis
	Quantity     int32  `json:"quantity"`
	ExecutedAt   int64  `json:"executed_at"`
	ContractID   string `json:"contract_id,omitempty"`
}

// NewTrade creates a new trade record
func NewTrade(buyOrderID, sellOrderID string, price uint64, quantity int32, contractID string) *Trade {
	return &Trade{
		ID:          uuid.New().String(),
		BuyOrderID:  buyOrderID,
		SellOrderID: sellOrderID,
		Price:       price,
		Quantity:    quantity,
		ExecutedAt:  time.Now().Unix(),
		ContractID:  contractID,
	}
}

// OrderBookEntry represents a single price level in the orderbook
type OrderBookEntry struct {
	Price    uint64 `json:"price"`
	Quantity int32  `json:"quantity"`
}

// OrderBook represents the current state of the order book for a specific contract specification
type OrderBook struct {
	Bids      []OrderBookEntry `json:"bids"` // Sorted by price descending
	Asks      []OrderBookEntry `json:"asks"` // Sorted by price ascending
	Timestamp int64            `json:"timestamp"`
}

// OrderRepository is the interface for order storage
type OrderRepository interface {
	// Save stores an order
	Save(order *Order) error
	
	// FindByID retrieves an order by ID
	FindByID(id string) (*Order, error)
	
	// FindOpenOrders retrieves open orders matching contract specifications
	FindOpenOrders(contractType ContractType, strikeHashRate float64, startBlockHeight, endBlockHeight int32) ([]*Order, error)
	
	// FindByUser retrieves orders for a specific user
	FindByUser(userID string, status OrderStatus, limit, offset int) ([]*Order, error)
	
	// CountByUser counts orders for a specific user
	CountByUser(userID string, status OrderStatus) (int, error)
	
	// FindAll retrieves all orders with optional pagination
	FindAll(limit, offset int) ([]*Order, error)
	
	// CountAll counts all orders
	CountAll() (int, error)
}

// TradeRepository is the interface for trade storage
type TradeRepository interface {
	// Save stores a trade
	Save(trade *Trade) error
	
	// FindByID retrieves a trade by ID
	FindByID(id string) (*Trade, error)
	
	// FindByContract retrieves trades for a specific contract specification
	FindByContract(contractType ContractType, strikeHashRate float64, startBlockHeight, endBlockHeight int32, limit, offset int) ([]*Trade, error)
	
	// CountByContract counts trades for a specific contract specification
	CountByContract(contractType ContractType, strikeHashRate float64, startBlockHeight, endBlockHeight int32) (int, error)
	
	// FindByOrder retrieves trades involving a specific order
	FindByOrder(orderID string) ([]*Trade, error)
	
	// FindAll retrieves all trades with optional pagination
	FindAll(limit, offset int) ([]*Trade, error)
	
	// CountAll counts all trades
	CountAll() (int, error)
}
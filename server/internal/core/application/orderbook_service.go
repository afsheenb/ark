package application

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/ark-network/ark/server/internal/core/domain"
	"github.com/ark-network/ark/server/internal/core/ports"
)

// OrderBookService manages the order book for hashrate derivatives
type OrderBookService struct {
	repos             ports.RepoManager
	contractService   *ContractService
	eventPublisher    OrderBookEventPublisher
	orderBooks        map[string]*OrderBook
	orderBooksMutex   sync.RWMutex
	tradingStatistics map[string]*MarketStatistics
	statsMutex        sync.RWMutex
}

// NewOrderBookService creates a new orderbook service
func NewOrderBookService(
	repos ports.RepoManager,
	contractService *ContractService,
	eventPublisher OrderBookEventPublisher,
) *OrderBookService {
	service := &OrderBookService{
		repos:             repos,
		contractService:   contractService,
		eventPublisher:    eventPublisher,
		orderBooks:        make(map[string]*OrderBook),
		tradingStatistics: make(map[string]*MarketStatistics),
	}

	// Load existing orders and rebuild the orderbook
	go service.loadExistingOrders()

	return service
}

// loadExistingOrders loads existing open orders from the database and rebuilds the orderbook
func (s *OrderBookService) loadExistingOrders() {
	// Get all orders
	orders, err := s.repos.OrderRepository().FindAll(0, 0)
	if err != nil {
		return
	}

	// Add open orders to the orderbook
	for _, order := range orders {
		if order.Status == domain.OrderStatusOpen || order.Status == domain.OrderStatusPartial {
			// Skip expired orders
			if order.IsExpired(time.Now().Unix()) {
				continue
			}

			// Get or create order book for this contract specification
			orderBook := s.getOrCreateOrderBook(
				order.ContractType,
				order.StrikeHashRate,
				order.StartBlockHeight,
				order.EndBlockHeight,
			)

			// Add order to the book
			switch order.Side {
			case domain.OrderSideBuy:
				orderBook.addBid(order)
			case domain.OrderSideSell:
				orderBook.addAsk(order)
			}
		}
	}
}

// PlaceOrder places a new order in the orderbook
func (s *OrderBookService) PlaceOrder(
	userID string,
	side domain.OrderSide,
	contractType domain.ContractType,
	strikeHashRate float64,
	startBlockHeight int32,
	endBlockHeight int32,
	price uint64,
	quantity int32,
	pubkey string,
	expiresAt int64,
) (*domain.Order, []*domain.Trade, error) {
	// Create new order
	order, err := domain.NewOrder(
		userID,
		side,
		contractType,
		strikeHashRate,
		startBlockHeight,
		endBlockHeight,
		price,
		quantity,
		pubkey,
		expiresAt,
	)
	if err != nil {
		return nil, nil, err
	}

	// Save the order
	if err := s.repos.OrderRepository().Save(order); err != nil {
		return nil, nil, fmt.Errorf("failed to save order: %w", err)
	}

	// Publish event
	s.eventPublisher.PublishOrderCreated(order)

	// Get or create order book for this contract specification
	orderBook := s.getOrCreateOrderBook(
		contractType,
		strikeHashRate,
		startBlockHeight,
		endBlockHeight,
	)

	// Try to match the order
	trades, err := s.matchOrder(order, orderBook)
	if err != nil {
		return nil, nil, err
	}

	// If order is not fully filled, add it to the book
	if order.Status == domain.OrderStatusOpen || order.Status == domain.OrderStatusPartial {
		switch order.Side {
		case domain.OrderSideBuy:
			orderBook.addBid(order)
		case domain.OrderSideSell:
			orderBook.addAsk(order)
		}
	}

	// Update market statistics
	if len(trades) > 0 {
		s.updateMarketStatistics(
			contractType,
			strikeHashRate,
			startBlockHeight,
			endBlockHeight,
			trades[len(trades)-1].Price, // Last trade price
			trades,
		)
	}

	return order, trades, nil
}

// CancelOrder cancels an existing order
func (s *OrderBookService) CancelOrder(orderID, userID string) error {
	// Get the order
	order, err := s.repos.OrderRepository().FindByID(orderID)
	if err != nil {
		return err
	}

	// Verify user owns the order
	if order.UserID != userID {
		return errors.New("user is not authorized to cancel this order")
	}

	// Cancel the order
	if err := order.Cancel(); err != nil {
		return err
	}

	// Save the updated order
	if err := s.repos.OrderRepository().Save(order); err != nil {
		return fmt.Errorf("failed to save canceled order: %w", err)
	}

	// Remove from the orderbook
	s.removeOrderFromBook(order)

	// Publish event
	s.eventPublisher.PublishOrderUpdated(order)

	return nil
}

// GetOrder gets an order by ID
func (s *OrderBookService) GetOrder(orderID string) (*domain.Order, error) {
	return s.repos.OrderRepository().FindByID(orderID)
}

// ListUserOrders lists orders for a specific user
func (s *OrderBookService) ListUserOrders(
	userID string,
	status domain.OrderStatus,
	limit, offset int,
) ([]*domain.Order, int, error) {
	orders, err := s.repos.OrderRepository().FindByUser(userID, status, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	count, err := s.repos.OrderRepository().CountByUser(userID, status)
	if err != nil {
		return nil, 0, err
	}

	return orders, count, nil
}

// GetOrderBook gets the current state of the order book for a specific contract
func (s *OrderBookService) GetOrderBook(
	contractType domain.ContractType,
	strikeHashRate float64,
	startBlockHeight int32,
	endBlockHeight int32,
	limit int,
) (*domain.OrderBook, error) {
	// Get the orderbook
	key := s.getOrderBookKey(contractType, strikeHashRate, startBlockHeight, endBlockHeight)
	
	s.orderBooksMutex.RLock()
	book, exists := s.orderBooks[key]
	s.orderBooksMutex.RUnlock()

	if !exists {
		// Create an empty orderbook if none exists
		return &domain.OrderBook{
			Bids:      []domain.OrderBookEntry{},
			Asks:      []domain.OrderBookEntry{},
			Timestamp: time.Now().Unix(),
		}, nil
	}

	// Convert to domain OrderBook and apply limit
	return book.toDomainOrderBook(limit), nil
}

// GetMarketData gets market data for a specific contract
func (s *OrderBookService) GetMarketData(
	contractType domain.ContractType,
	strikeHashRate float64,
	startBlockHeight int32,
	endBlockHeight int32,
) (*MarketStatistics, error) {
	key := s.getOrderBookKey(contractType, strikeHashRate, startBlockHeight, endBlockHeight)
	
	s.statsMutex.RLock()
	stats, exists := s.tradingStatistics[key]
	s.statsMutex.RUnlock()

	if !exists {
		// Create default statistics if none exist
		return &MarketStatistics{
			BestBid:     0,
			BestAsk:     0,
			LastPrice:   0,
			DailyVolume: 0,
			DailyChange: 0,
			Timestamp:   time.Now().Unix(),
		}, nil
	}

	return stats, nil
}

// ListTrades lists trades for a specific contract
func (s *OrderBookService) ListTrades(
	contractType domain.ContractType,
	strikeHashRate float64,
	startBlockHeight int32,
	endBlockHeight int32,
	limit, offset int,
) ([]*domain.Trade, int, error) {
	trades, err := s.repos.TradeRepository().FindByContract(
		contractType,
		strikeHashRate,
		startBlockHeight,
		endBlockHeight,
		limit,
		offset,
	)
	if err != nil {
		return nil, 0, err
	}

	count, err := s.repos.TradeRepository().CountByContract(
		contractType,
		strikeHashRate,
		startBlockHeight,
		endBlockHeight,
	)
	if err != nil {
		return nil, 0, err
	}

	return trades, count, nil
}

// matchOrder tries to match an order with existing orders in the book
func (s *OrderBookService) matchOrder(order *domain.Order, book *OrderBook) ([]*domain.Trade, error) {
	var trades []*domain.Trade
	var filledOrders []*domain.Order // Track filled orders for efficient removal

	// Create a transaction context with error tracking
	type txError struct {
		phase string
		err   error
	}
	var txErrors []txError

	// Determine which side to match against
	var matchPrices []uint64
	if order.Side == domain.OrderSideBuy {
		// Buy order matches against asks (sells), sorted by price ascending
		// Get sorted price levels for asks
		matchPrices = getSortedPrices(book.asksByPriceLevel, true) // ascending
	} else {
		// Sell order matches against bids (buys), sorted by price descending
		// Get sorted price levels for bids
		matchPrices = getSortedPrices(book.bidsByPriceLevel, false) // descending
	}
	
	// Enhanced matching algorithm using price levels for better performance
	// This is much more efficient for large orderbooks with many orders at the same price
	for _, price := range matchPrices {
		// Check if this price level can match
		if (order.Side == domain.OrderSideBuy && price > order.Price) ||
		   (order.Side == domain.OrderSideSell && price < order.Price) {
			// Cannot match at this price level
			continue
		}
		
		// Get all orders at this price level
		var ordersAtPriceLevel []string
		if order.Side == domain.OrderSideBuy {
			ordersAtPriceLevel = book.asksByPriceLevel[price]
		} else {
			ordersAtPriceLevel = book.bidsByPriceLevel[price]
		}
		
		// Match against orders at this price level (sorted by time)
		for _, orderID := range ordersAtPriceLevel {
			var matchOrder *domain.Order
			if order.Side == domain.OrderSideBuy {
				matchOrder = book.asks[orderID]
			} else {
				matchOrder = book.bids[orderID]
			}
			
			// Skip if not valid
			if matchOrder == nil {
				continue
			}
			
			// Check if the orders can match
			if !order.CanMatch(matchOrder) {
				continue
			}
			
			// Determine the matching price and quantity
			// We use the existing order's price (price-time priority)
			matchPrice := matchOrder.Price
			matchQuantity := min(order.RemainingQuantity, matchOrder.RemainingQuantity)

			// Create a contract for this trade
			contractID, err := s.createContract(order, matchOrder, matchPrice)
			if err != nil {
				// Record error but continue with other matches
				txErrors = append(txErrors, txError{
					phase: "create_contract",
					err:   err,
				})
				continue
			}

			// Create a trade record
			var buyOrderID, sellOrderID string
			if order.Side == domain.OrderSideBuy {
				buyOrderID, sellOrderID = order.ID, matchOrder.ID
			} else {
				buyOrderID, sellOrderID = matchOrder.ID, order.ID
			}

			trade := domain.NewTrade(buyOrderID, sellOrderID, matchPrice, matchQuantity, contractID)

			// Save the trade
			if err := s.repos.TradeRepository().Save(trade); err != nil {
				txErrors = append(txErrors, txError{
					phase: "save_trade",
					err:   err,
				})
				continue
			}

			// Update the orders
			if err := order.Fill(matchQuantity); err != nil {
				txErrors = append(txErrors, txError{
					phase: "fill_order",
					err:   err,
				})
				continue
			}
			if err := matchOrder.Fill(matchQuantity); err != nil {
				txErrors = append(txErrors, txError{
					phase: "fill_match_order",
					err:   err,
				})
				continue
			}

			// Save the updated orders
			if err := s.repos.OrderRepository().Save(order); err != nil {
				txErrors = append(txErrors, txError{
					phase: "save_order",
					err:   err,
				})
				continue
			}
			if err := s.repos.OrderRepository().Save(matchOrder); err != nil {
				txErrors = append(txErrors, txError{
					phase: "save_match_order",
					err:   err,
				})
				continue
			}

			// Publish events
			s.eventPublisher.PublishOrderUpdated(order)
			s.eventPublisher.PublishOrderUpdated(matchOrder)
			s.eventPublisher.PublishTradeExecuted(trade)

			// Add trade to results
			trades = append(trades, trade)

			// Track filled orders for efficient removal
			if matchOrder.Status == domain.OrderStatusFilled {
				filledOrders = append(filledOrders, matchOrder)
			}

			// If order is fully filled, stop matching and add to removal list
			if order.Status == domain.OrderStatusFilled {
				filledOrders = append(filledOrders, order)
				break
			}
		}
		
		// If order is fully filled, stop checking more price levels
		if order.Status == domain.OrderStatusFilled {
			break
		}
	}

	// Batch process filled orders removal for better performance
	if len(trades) > 0 {
		// First remove any filled orders we already have in memory
		for _, filledOrder := range filledOrders {
			s.removeOrderFromBook(filledOrder)
		}

		// Check for any other orders that might need removal but weren't tracked
		// This handles edge cases and ensures consistency
		orderIDsChecked := make(map[string]bool)
		for _, order := range filledOrders {
			orderIDsChecked[order.ID] = true
		}

		for _, trade := range trades {
			// Check if we need to remove the buy order
			if !orderIDsChecked[trade.BuyOrderID] {
				buyOrder, err := s.repos.OrderRepository().FindByID(trade.BuyOrderID)
				if err == nil && buyOrder.Status == domain.OrderStatusFilled {
					s.removeOrderFromBook(buyOrder)
				}
				orderIDsChecked[trade.BuyOrderID] = true
			}
			
			// Check if we need to remove the sell order
			if !orderIDsChecked[trade.SellOrderID] {
				sellOrder, err := s.repos.OrderRepository().FindByID(trade.SellOrderID)
				if err == nil && sellOrder.Status == domain.OrderStatusFilled {
					s.removeOrderFromBook(sellOrder)
				}
				orderIDsChecked[trade.SellOrderID] = true
			}
		}
	}

	// Log any errors that occurred during matching
	if len(txErrors) > 0 {
		// In a production system, we'd properly log these errors
		// fmt.Printf("Encountered %d errors during order matching\n", len(txErrors))
	}

	return trades, nil
}

// getSortedPrices returns a sorted slice of price levels
func getSortedPrices(priceMap map[uint64][]string, ascending bool) []uint64 {
	prices := make([]uint64, 0, len(priceMap))
	for price := range priceMap {
		prices = append(prices, price)
	}
	
	if ascending {
		sort.Slice(prices, func(i, j int) bool {
			return prices[i] < prices[j]
		})
	} else {
		sort.Slice(prices, func(i, j int) bool {
			return prices[i] > prices[j]
		})
	}
	
	return prices
}

// createContract creates a new contract based on a matched trade
func (s *OrderBookService) createContract(
	order1, order2 *domain.Order,
	price uint64,
) (string, error) {
	// Determine buyer and seller based on order sides
	var buyerOrder, sellerOrder *domain.Order
	if order1.Side == domain.OrderSideBuy {
		buyerOrder, sellerOrder = order1, order2
	} else {
		buyerOrder, sellerOrder = order2, order1
	}

	// Create contract
	contract, err := s.contractService.CreateContract(
		buyerOrder.ContractType,
		buyerOrder.StrikeHashRate,
		buyerOrder.StartBlockHeight,
		buyerOrder.EndBlockHeight,
		0, // Auto-calculate target timestamp
		price,
		price - sellerOrder.Price, // Premium is the difference between prices
		buyerOrder.Pubkey,
		sellerOrder.Pubkey,
		0, // No expiration for matched contracts
	)
	if err != nil {
		return "", err
	}

	return contract.ID, nil
}

// getOrCreateOrderBook gets or creates an order book for a specific contract
func (s *OrderBookService) getOrCreateOrderBook(
	contractType domain.ContractType,
	strikeHashRate float64,
	startBlockHeight int32,
	endBlockHeight int32,
) *OrderBook {
	key := s.getOrderBookKey(contractType, strikeHashRate, startBlockHeight, endBlockHeight)
	
	s.orderBooksMutex.RLock()
	book, exists := s.orderBooks[key]
	s.orderBooksMutex.RUnlock()

	if !exists {
		book = NewOrderBook()
		s.orderBooksMutex.Lock()
		s.orderBooks[key] = book
		s.orderBooksMutex.Unlock()
	}

	return book
}

// getOrderBookKey generates a unique key for an orderbook
func (s *OrderBookService) getOrderBookKey(
	contractType domain.ContractType,
	strikeHashRate float64,
	startBlockHeight int32,
	endBlockHeight int32,
) string {
	return fmt.Sprintf("%s-%.2f-%d-%d", contractType, strikeHashRate, startBlockHeight, endBlockHeight)
}

// removeOrderFromBook removes an order from the orderbook
func (s *OrderBookService) removeOrderFromBook(order *domain.Order) {
	key := s.getOrderBookKey(
		order.ContractType,
		order.StrikeHashRate,
		order.StartBlockHeight,
		order.EndBlockHeight,
	)
	
	s.orderBooksMutex.RLock()
	book, exists := s.orderBooks[key]
	s.orderBooksMutex.RUnlock()

	if !exists {
		return
	}

	// Remove the order
	switch order.Side {
	case domain.OrderSideBuy:
		book.removeBid(order.ID)
	case domain.OrderSideSell:
		book.removeAsk(order.ID)
	}
}

// updateMarketStatistics updates the market statistics for a contract
func (s *OrderBookService) updateMarketStatistics(
	contractType domain.ContractType,
	strikeHashRate float64,
	startBlockHeight int32,
	endBlockHeight int32,
	lastPrice uint64,
	newTrades []*domain.Trade,
) {
	key := s.getOrderBookKey(contractType, strikeHashRate, startBlockHeight, endBlockHeight)
	
	// Get existing stats
	s.statsMutex.RLock()
	stats, exists := s.tradingStatistics[key]
	s.statsMutex.RUnlock()

	if !exists {
		stats = &MarketStatistics{
			BestBid:     0,
			BestAsk:     0,
			LastPrice:   lastPrice,
			DailyVolume: 0,
			DailyChange: 0,
			Timestamp:   time.Now().Unix(),
		}
	}

	// Get best bid and ask
	orderBook := s.getOrCreateOrderBook(contractType, strikeHashRate, startBlockHeight, endBlockHeight)
	bestBid, bestAsk := orderBook.getBestPrices()

	// Calculate daily volume
	dailyVolume := stats.DailyVolume
	dailyStart := time.Now().Add(-24 * time.Hour).Unix()
	for _, trade := range newTrades {
		if trade.ExecutedAt >= dailyStart {
			dailyVolume += uint64(trade.Quantity) * trade.Price
		}
	}

	// Calculate price change (simplified)
	dailyChange := 0.0
	if stats.LastPrice > 0 {
		dailyChange = float64(lastPrice-stats.LastPrice) / float64(stats.LastPrice) * 100.0
	}

	// Update stats
	newStats := &MarketStatistics{
		BestBid:     bestBid,
		BestAsk:     bestAsk,
		LastPrice:   lastPrice,
		DailyVolume: dailyVolume,
		DailyChange: dailyChange,
		Timestamp:   time.Now().Unix(),
	}

	// Save updated stats
	s.statsMutex.Lock()
	s.tradingStatistics[key] = newStats
	s.statsMutex.Unlock()
}

// MarketStatistics represents market statistics for a specific contract
type MarketStatistics struct {
	BestBid     uint64  `json:"best_bid"`
	BestAsk     uint64  `json:"best_ask"`
	LastPrice   uint64  `json:"last_price"`
	DailyVolume uint64  `json:"daily_volume"`
	DailyChange float64 `json:"daily_change"`
	Timestamp   int64   `json:"timestamp"`
}

// OrderBook represents an in-memory order book
type OrderBook struct {
	bids             map[string]*domain.Order // Map of order ID to order
	asks             map[string]*domain.Order // Map of order ID to order
	bidsByPrice      []string                 // List of bid order IDs sorted by price (descending)
	asksByPrice      []string                 // List of ask order IDs sorted by price (ascending)
	bidsByPriceLevel map[uint64][]string      // Map of price to list of order IDs for faster matching
	asksByPriceLevel map[uint64][]string      // Map of price to list of order IDs for faster matching
	mu               sync.RWMutex
}

// NewOrderBook creates a new order book
func NewOrderBook() *OrderBook {
	return &OrderBook{
		bids:             make(map[string]*domain.Order),
		asks:             make(map[string]*domain.Order),
		bidsByPrice:      []string{},
		asksByPrice:      []string{},
		bidsByPriceLevel: make(map[uint64][]string),
		asksByPriceLevel: make(map[uint64][]string),
	}
}

// addBid adds a buy order to the book
func (b *OrderBook) addBid(order *domain.Order) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Add to bids map
	b.bids[order.ID] = order

	// Optimized insertion into sorted list (maintains price-time priority)
	// Find insertion point using binary search
	idx := sort.Search(len(b.bidsByPrice), func(i int) bool {
		bidI := b.bids[b.bidsByPrice[i]]
		if bidI.Price < order.Price {
			return true // Insert before this element (price descending)
		}
		if bidI.Price == order.Price && bidI.CreatedAt > order.CreatedAt {
			return true // Insert before this element (time ascending)
		}
		return false
	})

	// Insert at the correct position
	b.bidsByPrice = append(b.bidsByPrice, "")
	copy(b.bidsByPrice[idx+1:], b.bidsByPrice[idx:])
	b.bidsByPrice[idx] = order.ID
	
	// Add to price level map for faster matching
	priceLevel := order.Price
	b.bidsByPriceLevel[priceLevel] = append(b.bidsByPriceLevel[priceLevel], order.ID)
	
	// Keep the price level sorted by time
	if len(b.bidsByPriceLevel[priceLevel]) > 1 {
		sort.Slice(b.bidsByPriceLevel[priceLevel], func(i, j int) bool {
			bidI := b.bids[b.bidsByPriceLevel[priceLevel][i]]
			bidJ := b.bids[b.bidsByPriceLevel[priceLevel][j]]
			return bidI.CreatedAt < bidJ.CreatedAt
		})
	}
}

// addAsk adds a sell order to the book
func (b *OrderBook) addAsk(order *domain.Order) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Add to asks map
	b.asks[order.ID] = order

	// Optimized insertion into sorted list (maintains price-time priority)
	// Find insertion point using binary search
	idx := sort.Search(len(b.asksByPrice), func(i int) bool {
		askI := b.asks[b.asksByPrice[i]]
		if askI.Price > order.Price {
			return true // Insert before this element (price ascending)
		}
		if askI.Price == order.Price && askI.CreatedAt > order.CreatedAt {
			return true // Insert before this element (time ascending)
		}
		return false
	})

	// Insert at the correct position
	b.asksByPrice = append(b.asksByPrice, "")
	copy(b.asksByPrice[idx+1:], b.asksByPrice[idx:])
	b.asksByPrice[idx] = order.ID
	
	// Add to price level map for faster matching
	priceLevel := order.Price
	b.asksByPriceLevel[priceLevel] = append(b.asksByPriceLevel[priceLevel], order.ID)
	
	// Keep the price level sorted by time
	if len(b.asksByPriceLevel[priceLevel]) > 1 {
		sort.Slice(b.asksByPriceLevel[priceLevel], func(i, j int) bool {
			askI := b.asks[b.asksByPriceLevel[priceLevel][i]]
			askJ := b.asks[b.asksByPriceLevel[priceLevel][j]]
			return askI.CreatedAt < askJ.CreatedAt
		})
	}
}

// removeBid removes a buy order from the book
func (b *OrderBook) removeBid(orderID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Get order price (to remove from price level map)
	var orderPrice uint64
	order, exists := b.bids[orderID]
	if exists {
		orderPrice = order.Price
	}

	// Remove from bids map
	delete(b.bids, orderID)

	// Remove from sorted list using binary search
	// This is more efficient for large orderbooks
	n := len(b.bidsByPrice)
	idx := -1
	
	// Binary search to find the index (since we know it's sorted)
	// First find an approximate location using price
	if order != nil && n > 0 {
		// We can use price to narrow down the search
		left, right := 0, n-1
		for left <= right {
			mid := (left + right) / 2
			midOrder := b.bids[b.bidsByPrice[mid]]
			if midOrder.Price > order.Price {
				left = mid + 1
			} else if midOrder.Price < order.Price {
				right = mid - 1
			} else {
				// We found the price level, now search linearly within this level
				// This is typically a small search space
				for i := left; i <= right; i++ {
					if b.bidsByPrice[i] == orderID {
						idx = i
						break
					}
				}
				break
			}
		}
	}
	
	// Fall back to linear search if binary search failed
	if idx == -1 {
		for i, id := range b.bidsByPrice {
			if id == orderID {
				idx = i
				break
			}
		}
	}
	
	// Remove the order if found
	if idx != -1 {
		b.bidsByPrice = append(b.bidsByPrice[:idx], b.bidsByPrice[idx+1:]...)
	}
	
	// Remove from price level map
	if exists {
		priceLevelOrders := b.bidsByPriceLevel[orderPrice]
		for i, id := range priceLevelOrders {
			if id == orderID {
				// Remove this order ID from the slice
				b.bidsByPriceLevel[orderPrice] = append(
					priceLevelOrders[:i], 
					priceLevelOrders[i+1:]...
				)
				break
			}
		}
		
		// If no more orders at this price level, delete the map entry
		if len(b.bidsByPriceLevel[orderPrice]) == 0 {
			delete(b.bidsByPriceLevel, orderPrice)
		}
	}
}

// removeAsk removes a sell order from the book
func (b *OrderBook) removeAsk(orderID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Get order price (to remove from price level map)
	var orderPrice uint64
	order, exists := b.asks[orderID]
	if exists {
		orderPrice = order.Price
	}

	// Remove from asks map
	delete(b.asks, orderID)

	// Remove from sorted list using binary search
	// This is more efficient for large orderbooks
	n := len(b.asksByPrice)
	idx := -1
	
	// Binary search to find the index (since we know it's sorted)
	// First find an approximate location using price
	if order != nil && n > 0 {
		// We can use price to narrow down the search
		left, right := 0, n-1
		for left <= right {
			mid := (left + right) / 2
			midOrder := b.asks[b.asksByPrice[mid]]
			if midOrder.Price < order.Price {
				left = mid + 1
			} else if midOrder.Price > order.Price {
				right = mid - 1
			} else {
				// We found the price level, now search linearly within this level
				// This is typically a small search space
				for i := left; i <= right; i++ {
					if b.asksByPrice[i] == orderID {
						idx = i
						break
					}
				}
				break
			}
		}
	}
	
	// Fall back to linear search if binary search failed
	if idx == -1 {
		for i, id := range b.asksByPrice {
			if id == orderID {
				idx = i
				break
			}
		}
	}
	
	// Remove the order if found
	if idx != -1 {
		b.asksByPrice = append(b.asksByPrice[:idx], b.asksByPrice[idx+1:]...)
	}
	
	// Remove from price level map
	if exists {
		priceLevelOrders := b.asksByPriceLevel[orderPrice]
		for i, id := range priceLevelOrders {
			if id == orderID {
				// Remove this order ID from the slice
				b.asksByPriceLevel[orderPrice] = append(
					priceLevelOrders[:i], 
					priceLevelOrders[i+1:]...
				)
				break
			}
		}
		
		// If no more orders at this price level, delete the map entry
		if len(b.asksByPriceLevel[orderPrice]) == 0 {
			delete(b.asksByPriceLevel, orderPrice)
		}
	}
}

// getBids gets all buy orders sorted by price descending
func (b *OrderBook) getBids() []*domain.Order {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]*domain.Order, 0, len(b.bidsByPrice))
	for _, id := range b.bidsByPrice {
		if order, exists := b.bids[id]; exists {
			result = append(result, order)
		}
	}
	return result
}

// getAsks gets all sell orders sorted by price ascending
func (b *OrderBook) getAsks() []*domain.Order {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]*domain.Order, 0, len(b.asksByPrice))
	for _, id := range b.asksByPrice {
		if order, exists := b.asks[id]; exists {
			result = append(result, order)
		}
	}
	return result
}

// getBestPrices gets the best bid and ask prices
func (b *OrderBook) getBestPrices() (uint64, uint64) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var bestBid, bestAsk uint64
	if len(b.bidsByPrice) > 0 {
		if order, exists := b.bids[b.bidsByPrice[0]]; exists {
			bestBid = order.Price
		}
	}
	if len(b.asksByPrice) > 0 {
		if order, exists := b.asks[b.asksByPrice[0]]; exists {
			bestAsk = order.Price
		}
	}
	return bestBid, bestAsk
}

// toDomainOrderBook converts to a domain.OrderBook for API responses
func (b *OrderBook) toDomainOrderBook(limit int) *domain.OrderBook {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Aggregate orders by price level
	bidLevels := make(map[uint64]int32)
	askLevels := make(map[uint64]int32)

	// Aggregate bids
	for _, id := range b.bidsByPrice {
		if order, exists := b.bids[id]; exists {
			bidLevels[order.Price] += order.RemainingQuantity
		}
	}

	// Aggregate asks
	for _, id := range b.asksByPrice {
		if order, exists := b.asks[id]; exists {
			askLevels[order.Price] += order.RemainingQuantity
		}
	}

	// Convert to sorted slices
	bids := make([]domain.OrderBookEntry, 0, len(bidLevels))
	for price, quantity := range bidLevels {
		bids = append(bids, domain.OrderBookEntry{
			Price:    price,
			Quantity: quantity,
		})
	}
	sort.Slice(bids, func(i, j int) bool {
		return bids[i].Price > bids[j].Price // Sort by price descending
	})

	asks := make([]domain.OrderBookEntry, 0, len(askLevels))
	for price, quantity := range askLevels {
		asks = append(asks, domain.OrderBookEntry{
			Price:    price,
			Quantity: quantity,
		})
	}
	sort.Slice(asks, func(i, j int) bool {
		return asks[i].Price < asks[j].Price // Sort by price ascending
	})

	// Apply limit
	if limit > 0 {
		if len(bids) > limit {
			bids = bids[:limit]
		}
		if len(asks) > limit {
			asks = asks[:limit]
		}
	}

	return &domain.OrderBook{
		Bids:      bids,
		Asks:      asks,
		Timestamp: time.Now().Unix(),
	}
}

// Helper function for Go 1.20 compatibility (min() is available in Go 1.21+)
func min(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}

// OrderBookEventPublisher is an interface for publishing orderbook events
type OrderBookEventPublisher interface {
	// PublishOrderCreated publishes an order created event
	PublishOrderCreated(order *domain.Order)
	
	// PublishOrderUpdated publishes an order updated event
	PublishOrderUpdated(order *domain.Order)
	
	// PublishTradeExecuted publishes a trade executed event
	PublishTradeExecuted(trade *domain.Trade)
}
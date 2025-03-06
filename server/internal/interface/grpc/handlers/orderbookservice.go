package handlers

import (
	"context"
	"time"

	pb "github.com/ark-network/ark/api-spec/protobuf/gen/ark/v1"
	"github.com/ark-network/ark/server/internal/core/application"
	"github.com/ark-network/ark/server/internal/core/domain"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// OrderBookServiceHandler handles gRPC requests for the orderbook service
type OrderBookServiceHandler struct {
	pb.UnimplementedOrderBookServiceServer
	orderBookService *application.OrderBookService
}

// NewOrderBookServiceHandler creates a new orderbook service handler
func NewOrderBookServiceHandler(orderBookService *application.OrderBookService) *OrderBookServiceHandler {
	return &OrderBookServiceHandler{
		orderBookService: orderBookService,
	}
}

// PlaceOrder places a new order in the orderbook
func (h *OrderBookServiceHandler) PlaceOrder(
	ctx context.Context,
	req *pb.PlaceOrderRequest,
) (*pb.PlaceOrderResponse, error) {
	// Convert side from proto to domain
	var side domain.OrderSide
	switch req.Side {
	case pb.OrderSide_ORDER_SIDE_BUY:
		side = domain.OrderSideBuy
	case pb.OrderSide_ORDER_SIDE_SELL:
		side = domain.OrderSideSell
	default:
		return nil, status.Errorf(codes.InvalidArgument, "invalid order side")
	}

	// Convert contract type from proto to domain
	var contractType domain.ContractType
	switch req.ContractType {
	case pb.ContractType_CONTRACT_TYPE_CALL:
		contractType = domain.ContractTypeCall
	case pb.ContractType_CONTRACT_TYPE_PUT:
		contractType = domain.ContractTypePut
	default:
		return nil, status.Errorf(codes.InvalidArgument, "invalid contract type")
	}

	// Place order
	order, trades, err := h.orderBookService.PlaceOrder(
		req.UserId,
		side,
		contractType,
		req.StrikeHashRate,
		req.StartBlockHeight,
		req.EndBlockHeight,
		req.Price,
		req.Quantity,
		req.Pubkey,
		req.ExpiresAt,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to place order: %v", err)
	}

	// Convert domain order to proto
	pbOrder := domainOrderToProto(order)

	// Convert domain trades to proto
	pbTrades := make([]*pb.Trade, len(trades))
	for i, trade := range trades {
		pbTrades[i] = domainTradeToProto(trade)
	}

	return &pb.PlaceOrderResponse{
		Order:  pbOrder,
		Trades: pbTrades,
	}, nil
}

// CancelOrder cancels an existing order
func (h *OrderBookServiceHandler) CancelOrder(
	ctx context.Context,
	req *pb.CancelOrderRequest,
) (*pb.CancelOrderResponse, error) {
	// Cancel order
	err := h.orderBookService.CancelOrder(req.Id, req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to cancel order: %v", err)
	}

	return &pb.CancelOrderResponse{
		Success: true,
	}, nil
}

// GetOrderBook gets the current order book for a specific contract specification
func (h *OrderBookServiceHandler) GetOrderBook(
	ctx context.Context,
	req *pb.GetOrderBookRequest,
) (*pb.GetOrderBookResponse, error) {
	// Convert contract type from proto to domain
	var contractType domain.ContractType
	switch req.ContractType {
	case pb.ContractType_CONTRACT_TYPE_CALL:
		contractType = domain.ContractTypeCall
	case pb.ContractType_CONTRACT_TYPE_PUT:
		contractType = domain.ContractTypePut
	default:
		return nil, status.Errorf(codes.InvalidArgument, "invalid contract type")
	}

	// Get order book
	orderBook, err := h.orderBookService.GetOrderBook(
		contractType,
		req.StrikeHashRate,
		req.StartBlockHeight,
		req.EndBlockHeight,
		int(req.Limit),
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get order book: %v", err)
	}

	// Convert domain order book to proto
	pbOrderBook := &pb.OrderBook{
		Bids:      make([]*pb.OrderBookEntry, len(orderBook.Bids)),
		Asks:      make([]*pb.OrderBookEntry, len(orderBook.Asks)),
		Timestamp: orderBook.Timestamp,
	}

	for i, bid := range orderBook.Bids {
		pbOrderBook.Bids[i] = &pb.OrderBookEntry{
			Price:    bid.Price,
			Quantity: bid.Quantity,
		}
	}

	for i, ask := range orderBook.Asks {
		pbOrderBook.Asks[i] = &pb.OrderBookEntry{
			Price:    ask.Price,
			Quantity: ask.Quantity,
		}
	}

	return &pb.GetOrderBookResponse{
		Orderbook: pbOrderBook,
	}, nil
}

// GetOrder gets an order by ID
func (h *OrderBookServiceHandler) GetOrder(
	ctx context.Context,
	req *pb.GetOrderRequest,
) (*pb.GetOrderResponse, error) {
	// Get order
	order, err := h.orderBookService.GetOrder(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "order not found: %v", err)
	}

	// Convert domain order to proto
	pbOrder := domainOrderToProto(order)

	return &pb.GetOrderResponse{
		Order: pbOrder,
	}, nil
}

// ListUserOrders lists orders for a specific user
func (h *OrderBookServiceHandler) ListUserOrders(
	ctx context.Context,
	req *pb.ListUserOrdersRequest,
) (*pb.ListUserOrdersResponse, error) {
	// Convert status from proto to domain
	var orderStatus domain.OrderStatus
	switch req.Status {
	case pb.OrderStatus_ORDER_STATUS_OPEN:
		orderStatus = domain.OrderStatusOpen
	case pb.OrderStatus_ORDER_STATUS_PARTIAL:
		orderStatus = domain.OrderStatusPartial
	case pb.OrderStatus_ORDER_STATUS_FILLED:
		orderStatus = domain.OrderStatusFilled
	case pb.OrderStatus_ORDER_STATUS_CANCELED:
		orderStatus = domain.OrderStatusCanceled
	case pb.OrderStatus_ORDER_STATUS_EXPIRED:
		orderStatus = domain.OrderStatusExpired
	case pb.OrderStatus_ORDER_STATUS_UNSPECIFIED:
		orderStatus = "" // Empty string means no status filter
	default:
		return nil, status.Errorf(codes.InvalidArgument, "invalid order status")
	}

	// List user orders
	orders, total, err := h.orderBookService.ListUserOrders(
		req.UserId,
		orderStatus,
		int(req.Limit),
		int(req.Offset),
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list user orders: %v", err)
	}

	// Convert domain orders to proto
	pbOrders := make([]*pb.Order, len(orders))
	for i, order := range orders {
		pbOrders[i] = domainOrderToProto(order)
	}

	return &pb.ListUserOrdersResponse{
		Orders: pbOrders,
		Total:  int32(total),
	}, nil
}

// GetMarketData gets market data for a specific contract specification
func (h *OrderBookServiceHandler) GetMarketData(
	ctx context.Context,
	req *pb.GetMarketDataRequest,
) (*pb.GetMarketDataResponse, error) {
	// Convert contract type from proto to domain
	var contractType domain.ContractType
	switch req.ContractType {
	case pb.ContractType_CONTRACT_TYPE_CALL:
		contractType = domain.ContractTypeCall
	case pb.ContractType_CONTRACT_TYPE_PUT:
		contractType = domain.ContractTypePut
	default:
		return nil, status.Errorf(codes.InvalidArgument, "invalid contract type")
	}

	// Get market data
	marketData, err := h.orderBookService.GetMarketData(
		contractType,
		req.StrikeHashRate,
		req.StartBlockHeight,
		req.EndBlockHeight,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get market data: %v", err)
	}

	return &pb.GetMarketDataResponse{
		BestBid:     marketData.BestBid,
		BestAsk:     marketData.BestAsk,
		LastPrice:   marketData.LastPrice,
		DailyVolume: marketData.DailyVolume,
		DailyChange: marketData.DailyChange,
		Timestamp:   marketData.Timestamp,
	}, nil
}

// GetOrderBookEvents gets a stream of orderbook events
func (h *OrderBookServiceHandler) GetOrderBookEvents(
	req *pb.GetOrderBookEventsRequest,
	stream pb.OrderBookService_GetOrderBookEventsServer,
) error {
	// Channel for events
	eventCh := make(chan interface{})
	defer close(eventCh)
	
	// Subscribe to events
	// This assumes EventPublisher is accessible from the OrderBookService
	// If not, you'll need to modify this to receive an EventPublisher in the constructor
	// In a real implementation, you would use the EventPublisher to subscribe to events
	
	// Mock implementation - send some test events
	// In a real implementation, this would be replaced with real subscription logic
	go func() {
		time.Sleep(1 * time.Second)
		
		// Mock an order created event
		order := &domain.Order{
			ID:                "mock-order-id",
			UserID:            "mock-user-id",
			Side:              domain.OrderSideBuy,
			ContractType:      domain.ContractTypeCall,
			StrikeHashRate:    350.0,
			StartBlockHeight:  800000,
			EndBlockHeight:    802016,
			Price:             50000000,
			Quantity:          1,
			RemainingQuantity: 1,
			Status:            domain.OrderStatusOpen,
			Pubkey:            "mock-pubkey",
			CreatedAt:         time.Now().Unix(),
			UpdatedAt:         time.Now().Unix(),
		}
		
		eventCh <- &pb.OrderCreatedEvent{
			Order: domainOrderToProto(order),
		}
		
		time.Sleep(1 * time.Second)
		
		// Mock an order updated event
		order.Status = domain.OrderStatusPartial
		order.RemainingQuantity = 0
		order.UpdatedAt = time.Now().Unix()
		
		eventCh <- &pb.OrderUpdatedEvent{
			Order: domainOrderToProto(order),
		}
		
		time.Sleep(1 * time.Second)
		
		// Mock a trade executed event
		trade := &domain.Trade{
			ID:          "mock-trade-id",
			BuyOrderID:  "mock-order-id",
			SellOrderID: "mock-sell-order-id",
			Price:       50000000,
			Quantity:    1,
			ExecutedAt:  time.Now().Unix(),
			ContractID:  "mock-contract-id",
		}
		
		eventCh <- &pb.TradeExecutedEvent{
			Trade: domainTradeToProto(trade),
		}
	}()
	
	// Filter events by user ID if specified
	userID := req.UserId
	
	// Send events to client
	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-eventCh:
			var pbEvent *pb.GetOrderBookEventsResponse
			var skipEvent bool
			
			switch e := event.(type) {
			case *pb.OrderCreatedEvent:
				// Filter by user ID if specified
				if userID != "" && e.Order.UserId != userID {
					skipEvent = true
					break
				}
				
				pbEvent = &pb.GetOrderBookEventsResponse{
					Event: &pb.GetOrderBookEventsResponse_OrderCreated{
						OrderCreated: e,
					},
				}
			case *pb.OrderUpdatedEvent:
				// Filter by user ID if specified
				if userID != "" && e.Order.UserId != userID {
					skipEvent = true
					break
				}
				
				pbEvent = &pb.GetOrderBookEventsResponse{
					Event: &pb.GetOrderBookEventsResponse_OrderUpdated{
						OrderUpdated: e,
					},
				}
			case *pb.TradeExecutedEvent:
				// For trades, we don't filter by user ID in this example
				// In a real implementation, you would need to check if the user is involved in the trade
				
				pbEvent = &pb.GetOrderBookEventsResponse{
					Event: &pb.GetOrderBookEventsResponse_TradeExecuted{
						TradeExecuted: e,
					},
				}
			default:
				skipEvent = true
			}
			
			if skipEvent {
				continue
			}
			
			if err := stream.Send(pbEvent); err != nil {
				return err
			}
		}
	}
}

// ListTrades lists trades for a specific contract specification
func (h *OrderBookServiceHandler) ListTrades(
	ctx context.Context,
	req *pb.ListTradesRequest,
) (*pb.ListTradesResponse, error) {
	// Convert contract type from proto to domain
	var contractType domain.ContractType
	switch req.ContractType {
	case pb.ContractType_CONTRACT_TYPE_CALL:
		contractType = domain.ContractTypeCall
	case pb.ContractType_CONTRACT_TYPE_PUT:
		contractType = domain.ContractTypePut
	default:
		return nil, status.Errorf(codes.InvalidArgument, "invalid contract type")
	}

	// List trades
	trades, total, err := h.orderBookService.ListTrades(
		contractType,
		req.StrikeHashRate,
		req.StartBlockHeight,
		req.EndBlockHeight,
		int(req.Limit),
		int(req.Offset),
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list trades: %v", err)
	}

	// Convert domain trades to proto
	pbTrades := make([]*pb.Trade, len(trades))
	for i, trade := range trades {
		pbTrades[i] = domainTradeToProto(trade)
	}

	return &pb.ListTradesResponse{
		Trades: pbTrades,
		Total:  int32(total),
	}, nil
}

// domainOrderToProto converts a domain order to a proto order
func domainOrderToProto(order *domain.Order) *pb.Order {
	// Convert side
	var side pb.OrderSide
	switch order.Side {
	case domain.OrderSideBuy:
		side = pb.OrderSide_ORDER_SIDE_BUY
	case domain.OrderSideSell:
		side = pb.OrderSide_ORDER_SIDE_SELL
	default:
		side = pb.OrderSide_ORDER_SIDE_UNSPECIFIED
	}

	// Convert contract type
	var contractType pb.ContractType
	switch order.ContractType {
	case domain.ContractTypeCall:
		contractType = pb.ContractType_CONTRACT_TYPE_CALL
	case domain.ContractTypePut:
		contractType = pb.ContractType_CONTRACT_TYPE_PUT
	default:
		contractType = pb.ContractType_CONTRACT_TYPE_UNSPECIFIED
	}

	// Convert status
	var status pb.OrderStatus
	switch order.Status {
	case domain.OrderStatusOpen:
		status = pb.OrderStatus_ORDER_STATUS_OPEN
	case domain.OrderStatusPartial:
		status = pb.OrderStatus_ORDER_STATUS_PARTIAL
	case domain.OrderStatusFilled:
		status = pb.OrderStatus_ORDER_STATUS_FILLED
	case domain.OrderStatusCanceled:
		status = pb.OrderStatus_ORDER_STATUS_CANCELED
	case domain.OrderStatusExpired:
		status = pb.OrderStatus_ORDER_STATUS_EXPIRED
	default:
		status = pb.OrderStatus_ORDER_STATUS_UNSPECIFIED
	}

	return &pb.Order{
		Id:                order.ID,
		UserId:            order.UserID,
		Side:              side,
		ContractType:      contractType,
		StrikeHashRate:    order.StrikeHashRate,
		StartBlockHeight:  order.StartBlockHeight,
		EndBlockHeight:    order.EndBlockHeight,
		Price:             order.Price,
		Quantity:          int32(order.Quantity),
		RemainingQuantity: int32(order.RemainingQuantity),
		Status:            status,
		Pubkey:            order.Pubkey,
		CreatedAt:         order.CreatedAt,
		UpdatedAt:         order.UpdatedAt,
		ExpiresAt:         order.ExpiresAt,
	}
}

// domainTradeToProto converts a domain trade to a proto trade
func domainTradeToProto(trade *domain.Trade) *pb.Trade {
	return &pb.Trade{
		Id:          trade.ID,
		BuyOrderId:  trade.BuyOrderID,
		SellOrderId: trade.SellOrderID,
		Price:       trade.Price,
		Quantity:    trade.Quantity,
		ExecutedAt:  trade.ExecutedAt,
		ContractId:  trade.ContractID,
	}
}
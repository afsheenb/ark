package db

import (
	"sync"
	
	"github.com/ark-network/ark/server/internal/core/application"
	"github.com/ark-network/ark/server/internal/core/domain"
)

// EventPublisher is an implementation of application.EventPublisher
type EventPublisher struct {
	mu      sync.RWMutex
	handlers map[string][]func(interface{})
}

// NewEventPublisher creates a new event publisher
func NewEventPublisher() *EventPublisher {
	return &EventPublisher{
		handlers: make(map[string][]func(interface{})),
	}
}

// PublishContractCreated publishes a contract created event
func (p *EventPublisher) PublishContractCreated(contract *domain.HashrateContract) {
	p.publish("contract.created", contract)
}

// PublishContractSetup publishes a contract setup event
func (p *EventPublisher) PublishContractSetup(contractID, setupTxid string) {
	p.publish("contract.setup", map[string]string{
		"contract_id": contractID,
		"setup_txid":  setupTxid,
	})
}

// PublishContractSettled publishes a contract settled event
func (p *EventPublisher) PublishContractSettled(contractID, settlementTxid, winnerPubkey string) {
	p.publish("contract.settled", map[string]string{
		"contract_id":    contractID,
		"settlement_txid": settlementTxid,
		"winner_pubkey":  winnerPubkey,
	})
}

// PublishOrderCreated publishes an order created event
func (p *EventPublisher) PublishOrderCreated(order *domain.Order) {
	p.publish("order.created", order)
}

// PublishOrderUpdated publishes an order updated event
func (p *EventPublisher) PublishOrderUpdated(order *domain.Order) {
	p.publish("order.updated", order)
}

// PublishTradeExecuted publishes a trade executed event
func (p *EventPublisher) PublishTradeExecuted(trade *domain.Trade) {
	p.publish("trade.executed", trade)
}

// Subscribe subscribes to an event type
func (p *EventPublisher) Subscribe(eventType string, handler func(interface{})) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.handlers[eventType] = append(p.handlers[eventType], handler)
}

// publish publishes an event to all handlers
func (p *EventPublisher) publish(eventType string, data interface{}) {
	p.mu.RLock()
	handlers := p.handlers[eventType]
	p.mu.RUnlock()
	
	for _, handler := range handlers {
		go handler(data)
	}
}

// Ensure EventPublisher implements the required interfaces
var _ application.EventPublisher = (*EventPublisher)(nil)
var _ application.OrderBookEventPublisher = (*EventPublisher)(nil)
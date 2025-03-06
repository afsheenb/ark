package application

import (
	"sync"

	"github.com/ark-network/ark/server/internal/core/domain"
	log "github.com/sirupsen/logrus"
)

// EventPublisherImpl is an implementation of EventPublisher interface
type EventPublisherImpl struct {
	mu       sync.RWMutex
	handlers map[string][]func(interface{})
}

// NewEventPublisher creates a new event publisher
func NewEventPublisher() *EventPublisherImpl {
	return &EventPublisherImpl{
		handlers: make(map[string][]func(interface{})),
	}
}

// PublishContractCreated publishes a contract created event
func (p *EventPublisherImpl) PublishContractCreated(contract *domain.HashrateContract) {
	p.publish("contract.created", contract)
}

// PublishContractSetup publishes a contract setup event
func (p *EventPublisherImpl) PublishContractSetup(contractID, setupTxid string) {
	p.publish("contract.setup", map[string]string{
		"contract_id": contractID,
		"setup_txid":  setupTxid,
	})
}

// PublishContractSettled publishes a contract settled event
func (p *EventPublisherImpl) PublishContractSettled(contractID, settlementTxid, winnerPubkey string) {
	p.publish("contract.settled", map[string]string{
		"contract_id":    contractID,
		"settlement_txid": settlementTxid,
		"winner_pubkey":  winnerPubkey,
	})
}

// Subscribe adds a handler for a specific event type
func (p *EventPublisherImpl) Subscribe(eventType string, handler func(interface{})) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.handlers[eventType] = append(p.handlers[eventType], handler)
}

// publish handles sending events to registered handlers
func (p *EventPublisherImpl) publish(eventType string, data interface{}) {
	p.mu.RLock()
	handlers := p.handlers[eventType]
	p.mu.RUnlock()
	
	if len(handlers) == 0 {
		log.Debugf("No handlers for event type: %s", eventType)
		return
	}
	
	for _, handler := range handlers {
		go func(h func(interface{}), d interface{}) {
			defer func() {
				if r := recover(); r != nil {
					log.Errorf("Panic in event handler for %s: %v", eventType, r)
				}
			}()
			h(d)
		}(handler, data)
	}
}

// PublishOrderCreated publishes an order created event
func (p *EventPublisherImpl) PublishOrderCreated(order *domain.Order) {
	p.publish("order.created", order)
}

// PublishOrderUpdated publishes an order updated event
func (p *EventPublisherImpl) PublishOrderUpdated(order *domain.Order) {
	p.publish("order.updated", order)
}

// PublishTradeExecuted publishes a trade executed event
func (p *EventPublisherImpl) PublishTradeExecuted(trade *domain.Trade) {
	p.publish("trade.executed", trade)
}

// Ensure EventPublisherImpl implements the EventPublisher interface
var _ EventPublisher = (*EventPublisherImpl)(nil)
// Ensure EventPublisherImpl implements the OrderBookEventPublisher interface
var _ OrderBookEventPublisher = (*EventPublisherImpl)(nil)
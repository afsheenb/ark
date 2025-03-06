# HashHedge Hashrate Derivatives Implementation Summary

This document summarizes the key changes and additions made to the ASP daemon to implement Bitcoin hashrate derivatives functionality.

## Overview

The implementation adds support for trading and settlement of Bitcoin hashrate options contracts, allowing users to hedge against or speculate on Bitcoin network hashrate changes. The implementation includes:

1. Contract management system
2. Hashrate calculation infrastructure
3. Order book and matching engine
4. Database storage for contracts and orders
5. RPC interfaces for all operations

## Core Components

### 1. Hashrate Contracts Module

A complete system for managing hashrate derivative contracts:

- **Contract Types**: Binary call and put options based on Bitcoin hashrate measurements
- **Contract Parameters**:
  - Strike hashrate (in EH/s - exahash per second)
  - Start and end block heights for measurement period
  - Target settlement timestamp
  - Contract size in satoshis
  - Premium amount paid by buyer to seller
  - Both buyer and seller public keys

- **Contract Lifecycle**:
  - Creation: Define contract terms and parties
  - Setup: Lock funds in multisig arrangement
  - Settlement: Determine winner based on actual hashrate measurement
  - Optional early settlement if outcome is clearly determined

- **Settlement Logic**:
  - Call option: Buyer wins if actual hashrate > strike hashrate
  - Put option: Buyer wins if actual hashrate < strike hashrate
  - Winner receives contract size amount
  - Uses VTXO system for efficient settlement

### 2. Hashrate Calculation

- Real-time Bitcoin network hashrate calculation
- Historical hashrate data retrieval and storage
- Configurable window size for averaging (default 1008 blocks/~1 week)
- API endpoints for current and historical hashrate data

### 3. Order Book System

A complete trading system for hashrate derivatives:

- **Order Types**:
  - Buy (bid) and sell (ask) orders
  - Market and limit orders
  - Optional expiration time

- **Order Book Features**:
  - Price-time priority matching
  - Partial and complete fills
  - Market data calculations
  - Order lifecycle management (open, partial, filled, cancelled, expired)

- **Trade Execution**:
  - Automatic matching based on price and time priority
  - Trade history tracking
  - Market statistics collection (volume, best prices, etc.)

### 4. Database Schema

#### Contracts Table

```sql
CREATE TABLE contracts (
    id UUID PRIMARY KEY,
    contract_type TEXT NOT NULL,
    strike_hash_rate DOUBLE PRECISION NOT NULL,
    start_block_height INTEGER NOT NULL,
    end_block_height INTEGER NOT NULL,
    target_timestamp BIGINT NOT NULL,
    contract_size BIGINT NOT NULL,
    premium BIGINT NOT NULL,
    buyer_pubkey BYTEA NOT NULL,
    seller_pubkey BYTEA NOT NULL,
    status TEXT NOT NULL,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    expires_at BIGINT,
    setup_txid TEXT,
    final_txid TEXT,
    settlement_txid TEXT
);
```

#### Orders Table

```sql
CREATE TABLE orders (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    side TEXT NOT NULL,
    contract_type TEXT NOT NULL,
    strike_hash_rate DOUBLE PRECISION NOT NULL,
    start_block_height INTEGER NOT NULL,
    end_block_height INTEGER NOT NULL,
    price BIGINT NOT NULL,
    quantity INTEGER NOT NULL,
    remaining_quantity INTEGER NOT NULL,
    status TEXT NOT NULL,
    pubkey BYTEA NOT NULL,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    expires_at BIGINT
);
```

#### Trades Table

```sql
CREATE TABLE trades (
    id UUID PRIMARY KEY,
    buy_order_id UUID NOT NULL,
    sell_order_id UUID NOT NULL,
    price BIGINT NOT NULL,
    quantity INTEGER NOT NULL,
    executed_at BIGINT NOT NULL
);
```

## RPC API Endpoints

### Contract Service API

```protobuf
service ContractService {
    rpc CreateContract(CreateContractRequest) returns (CreateContractResponse) {}
    rpc GetContract(GetContractRequest) returns (GetContractResponse) {}
    rpc SetupContract(SetupContractRequest) returns (SetupContractResponse) {}
    rpc SettleContract(SettleContractRequest) returns (SettleContractResponse) {}
    rpc ListContracts(ListContractsRequest) returns (ListContractsResponse) {}
    rpc GetHashrate(GetHashrateRequest) returns (GetHashrateResponse) {}
    rpc GetHistoricalHashrates(GetHistoricalHashratesRequest) returns (GetHistoricalHashratesResponse) {}
}
```

### Order Book Service API

```protobuf
service OrderBookService {
    rpc PlaceOrder(PlaceOrderRequest) returns (PlaceOrderResponse) {}
    rpc CancelOrder(CancelOrderRequest) returns (CancelOrderResponse) {}
    rpc GetOrderBook(GetOrderBookRequest) returns (GetOrderBookResponse) {}
    rpc GetOrder(GetOrderRequest) returns (GetOrderResponse) {}
    rpc ListUserOrders(ListUserOrdersRequest) returns (ListUserOrdersResponse) {}
    rpc GetMarketData(GetMarketDataRequest) returns (GetMarketDataResponse) {}
}
```

## CLI Command Interface

New command-line interfaces were added to interact with the contract and order book systems:

### Contract Commands

```
aspd contract create --contract_type <CALL|PUT> --strike_hash_rate <RATE> --start_block_height <HEIGHT> --end_block_height <HEIGHT> --contract_size <SATS> --premium <SATS> --buyer_pubkey <KEY> --seller_pubkey <KEY>
aspd contract get <ID>
aspd contract list [--status <STATUS>] [--limit <N>] [--offset <N>]
aspd contract checkSettlement <ID>
aspd contract settle <ID>
aspd contract getHashRate
```

### Order Book Commands

```
aspd orderbook placeOrder --user_id <ID> --side <BUY|SELL> --contract_type <CALL|PUT> --strike_hash_rate <RATE> --start_block_height <HEIGHT> --end_block_height <HEIGHT> --price <SATS> --quantity <N> --pubkey <KEY> [--expires_in <SECONDS>]
aspd orderbook cancelOrder <ID>
aspd orderbook getOrder <ID>
aspd orderbook listUserOrders <USER_ID> [--limit <N>] [--offset <N>]
aspd orderbook getOrderBook --contract_type <CALL|PUT> --strike_hash_rate <RATE> --start_block_height <HEIGHT> --end_block_height <HEIGHT> [--limit <N>]
aspd orderbook getStats
```

## Implementation Notes

1. The system uses the existing VTXO (Virtual UTXO) framework for settlements
2. The RPC server architecture was extended to support the new services
3. Error handling specifically for contracts and order book operations was added
4. Telemetry/metrics were added to track contract operations and market activity
5. Command-line interfaces were extended to support contract and order management
6. Validation checks ensure contracts and orders meet system requirements
7. Historical hashrate data is stored for settlement verification

## Integration Requirements for Go Implementation

When implementing this functionality in Go:

1. Ensure Bitcoin hashrate calculation matches the exact algorithm used here
2. Maintain the same contract lifecycle states and status transitions
3. Implement the order matching algorithm with the same price-time priority
4. Keep the same database schema structure for compatibility
5. Match the RPC interfaces exactly for client compatibility
6. Implement the same validation rules for contracts and orders
7. Use the existing VTXO system for settlements
8. Add appropriate metrics and monitoring for contract operations
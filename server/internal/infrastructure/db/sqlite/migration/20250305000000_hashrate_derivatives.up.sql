-- Create hashrate contracts table
CREATE TABLE hashrate_contracts (
    id TEXT PRIMARY KEY,
    contract_type TEXT NOT NULL,
    strike_hash_rate REAL NOT NULL,
    start_block_height INTEGER NOT NULL,
    end_block_height INTEGER NOT NULL,
    target_timestamp INTEGER NOT NULL,
    contract_size INTEGER NOT NULL,
    premium INTEGER NOT NULL,
    buyer_pubkey TEXT NOT NULL,
    seller_pubkey TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    expires_at INTEGER,
    setup_txid TEXT,
    final_txid TEXT,
    settlement_txid TEXT
);

-- Create indexes for hashrate contracts
CREATE INDEX idx_hashrate_contracts_status ON hashrate_contracts(status);
CREATE INDEX idx_hashrate_contracts_buyer_pubkey ON hashrate_contracts(buyer_pubkey);
CREATE INDEX idx_hashrate_contracts_seller_pubkey ON hashrate_contracts(seller_pubkey);

-- Create orders table
CREATE TABLE orders (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    side TEXT NOT NULL,
    contract_type TEXT NOT NULL,
    strike_hash_rate REAL NOT NULL,
    start_block_height INTEGER NOT NULL,
    end_block_height INTEGER NOT NULL,
    price INTEGER NOT NULL,
    quantity INTEGER NOT NULL,
    remaining_quantity INTEGER NOT NULL,
    status TEXT NOT NULL,
    pubkey TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    expires_at INTEGER
);

-- Create indexes for orders
CREATE INDEX idx_orders_status ON orders(status);
CREATE INDEX idx_orders_user_id ON orders(user_id);
CREATE INDEX idx_orders_contract_spec ON orders(contract_type, strike_hash_rate, start_block_height, end_block_height);

-- Create trades table
CREATE TABLE trades (
    id TEXT PRIMARY KEY,
    buy_order_id TEXT NOT NULL,
    sell_order_id TEXT NOT NULL,
    price INTEGER NOT NULL,
    quantity INTEGER NOT NULL,
    executed_at INTEGER NOT NULL,
    contract_id TEXT NOT NULL
);

-- Create indexes for trades
CREATE INDEX idx_trades_buy_order_id ON trades(buy_order_id);
CREATE INDEX idx_trades_sell_order_id ON trades(sell_order_id);
CREATE INDEX idx_trades_contract_id ON trades(contract_id);

-- Create historical hashrates table
CREATE TABLE historical_hashrates (
    block_height INTEGER PRIMARY KEY,
    timestamp INTEGER NOT NULL,
    hashrate REAL NOT NULL,
    difficulty INTEGER NOT NULL
);

-- Create index for historical hashrates
CREATE INDEX idx_historical_hashrates_timestamp ON historical_hashrates(timestamp);
[![Release](https://img.shields.io/github/release/thrownew/go-binance-orderbook.svg)](https://github.com/thrownew/go-binance-orderbook/releases/latest)
[![License](https://img.shields.io/github/license/thrownew/go-binance-orderbook.svg)](https://raw.githubusercontent.com/thrownew/go-binance-orderbook/master/LICENSE)
[![Godocs](https://img.shields.io/badge/godoc-reference-blue.svg)](https://godoc.org/github.com/thrownew/go-binance-orderbook)

[![Build Status](https://github.com/thrownew/go-binance-orderbook/workflows/CI/badge.svg)](https://github.com/thrownew/go-binance-orderbook/actions)
[![codecov](https://codecov.io/gh/thrownew/go-binance-orderbook/branch/master/graph/badge.svg)](https://codecov.io/gh/thrownew/go-binance-orderbook)
[![Go Report Card](https://goreportcard.com/badge/github.com/thrownew/go-binance-orderbook)](https://goreportcard.com/report/github.com/thrownew/go-binance-orderbook)

# go-binance-orderbook

Binance OrderBook based on [USDⓈ-M Futures](https://developers.binance.com/docs/derivatives/usds-margined-futures/general-info)

A high-performance orderbook builder that maintains real-time orderbook state for multiple trading symbols by subscribing to Binance WebSocket streams and applying incremental updates. The builder automatically handles snapshot synchronization, event validation, and memory-efficient depth management.

## Installation

```bash
go get github.com/thrownew/go-binance-orderbook
```

## Usage

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	orderbook "github.com/thrownew/go-binance-orderbook"
)

func main() {
	b := orderbook.NewBuilder(
		orderbook.WithSymbols("BTCUSDT", "ETHUSDT"),
		orderbook.WithHandleDepth(3),
		orderbook.WithHandler(func(ob orderbook.OrderBook) {
			fmt.Printf("update: %+v\n", ob)
		}),
		orderbook.WithLogger(orderbook.NewSlogLogger(slog.With("component", "binance-orderbook"))),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		if err := b.Run(ctx); err != nil {
			fmt.Println(fmt.Errorf("run error: %w", err))
		}
	}()

	<-time.After(10 * time.Second)

	cancel()
}
```

## How It Works

The builder maintains real-time orderbook state by combining WebSocket stream updates with periodic snapshot synchronization. Here's a brief overview:

1. **Initialization**: Creates separate orderbook instances and event channels for each symbol
2. **WebSocket Connection**: Establishes a single WebSocket connection to Binance's combined stream endpoint
3. **Snapshot Loading**: Fetches initial orderbook snapshots via HTTP when needed (on first event or synchronization errors)
4. **Event Processing**: Applies incremental depth updates from WebSocket events to maintain current state
5. **Memory Management**: Stores only a configurable number of depth levels to optimize memory usage
6. **Synchronization**: Validates update ID chains to ensure data integrity and automatically resyncs when needed

### Detailed Architecture

#### WebSocket Connection

The builder establishes a single WebSocket connection to `wss://fstream.binance.com/stream` with a combined stream format that includes all configured symbols. Each symbol subscribes to its depth stream (e.g., `btcusdt@depth@500ms`). The connection runs in a separate goroutine and automatically reconnects on errors with exponential backoff.

**Stream Format**: The WebSocket receives events in the format `{stream: "btcusdt@depth@500ms", data: {...}}`, where `data` contains the depth update with:
- `U`: First update ID in the event
- `u`: Final update ID in the event  
- `pu`: Previous final update ID (for chain validation)
- `b`: Bid updates as `[price, amount]` pairs
- `a`: Ask updates as `[price, amount]` pairs

#### Snapshot Loading

Orderbook snapshots are fetched via HTTP from `https://fapi.binance.com/fapi/v1/depth` when:
- The first depth event arrives (book is empty)
- Event application fails due to synchronization issues
- Update ID chain validation fails

The snapshot provides the initial state with a `lastUpdateId` that must be synchronized with subsequent WebSocket events. A semaphore limits concurrent snapshot fetches (default: 4) to prevent rate limiting and memory spikes.

#### Event Application

Each symbol has a dedicated goroutine that processes events from its channel. When an event arrives:

1. **Validation**: Checks if the book is initialized (has a `lastUpdateId`)
2. **Old Event Filtering**: Skips events with `UFinal < lastUpdateID`
3. **Chain Validation**: If synchronized, verifies `PrevFinal == lastUpdateID` to ensure continuity
4. **Synchronization Detection**: Marks as synchronized when `U <= lastUpdateID <= UFinal`
5. **Level Updates**: Applies bid/ask updates:
   - If amount > 0: adds or updates the price level
   - If amount = 0: removes the price level
6. **Memory Trimming**: After updates, sorts all levels and removes excess levels beyond `storeDepth` (default: 500)

#### Depth Management

The builder maintains three configurable depth parameters:

- **`storeDepth`** (default: 500): Maximum number of price levels stored in memory per side (bids/asks). Excess levels are trimmed after each update to prevent memory growth.
- **`snapshotDepth`** (default: 1000): Number of levels requested in snapshot API calls. Should be >= `storeDepth`.
- **`handleDepth`** (default: 100): Number of levels included in handler callbacks. This allows storing more levels than what's passed to handlers.

#### Synchronization Logic

The builder uses Binance's update ID system to ensure data integrity:

- Each depth event contains `U` (first ID) and `u` (final ID) representing a range of updates
- Events also include `pu` (previous final ID) to validate the chain
- When `pu == lastUpdateID`, the chain is valid
- When `U <= lastUpdateID <= u`, the book becomes synchronized
- If the chain breaks (`pu != lastUpdateID` when synchronized), a new snapshot is fetched

#### Thread Safety

Each orderbook instance uses `sync.RWMutex` to protect concurrent access:
- Write operations (applying events, snapshots) use `Lock()`
- Read operations (scraping orderbook) use `RLock()`
- Event channels are buffered (default: 1024) to prevent blocking

#### Error Handling

- **WebSocket errors**: Logged and trigger reconnection after 1 second delay
- **Event application errors**: Trigger snapshot reload and event reapplication
- **Snapshot fetch errors**: Logged; processing continues with next event
- **Invalid events**: Skipped with warning logs
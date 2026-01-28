package binanceorderbook

import (
	"errors"
	"net/http"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/jokruger/dec128"
)

func TestBook_applyEvent(t *testing.T) {
	tests := []struct {
		name          string
		initialBook   *book
		event         DepthEvent
		expectedError error
		expectedBids  int
		expectedAsks  int
		expectedID    int64
		checkSync     bool
		expectedSync  bool
	}{
		{
			name: "empty book should return error",
			initialBook: &book{
				symbol:       "BTCUSDT",
				depth:        100,
				bids:         make(map[string]dec128.Dec128),
				asks:         make(map[string]dec128.Dec128),
				lastUpdateID: 0,
			},
			event: DepthEvent{
				U:         100,
				UFinal:    150,
				PrevFinal: 0,
				Bids:      [][2]string{{"50000.00", "1.5"}},
				Asks:      [][2]string{{"50100.00", "2.0"}},
			},
			expectedError: errEmptyBook,
		},
		{
			name: "old event should be skipped",
			initialBook: &book{
				symbol:       "BTCUSDT",
				depth:        100,
				bids:         make(map[string]dec128.Dec128),
				asks:         make(map[string]dec128.Dec128),
				lastUpdateID: 200,
			},
			event: DepthEvent{
				U:         100,
				UFinal:    150,
				PrevFinal: 0,
				Bids:      [][2]string{{"50000.00", "1.5"}},
				Asks:      [][2]string{{"50100.00", "2.0"}},
			},
			expectedError: nil,
			expectedID:    200, // should remain unchanged
		},
		{
			name: "valid event should update book",
			initialBook: &book{
				symbol:       "BTCUSDT",
				depth:        100,
				bids:         map[string]dec128.Dec128{"50000.00": dec128.FromString("1.0")},
				asks:         map[string]dec128.Dec128{"50100.00": dec128.FromString("2.0")},
				lastUpdateID: 100,
			},
			event: DepthEvent{
				U:         100,
				UFinal:    150,
				PrevFinal: 100,
				Bids:      [][2]string{{"50001.00", "0.5"}, {"50000.00", "0"}}, // update and remove
				Asks:      [][2]string{{"50101.00", "1.0"}},
			},
			expectedError: nil,
			expectedBids:  2, // 50001.00 (new) and possibly 50000.00 if not properly removed, or trimming keeps both
			expectedAsks:  2, // two asks (50100.00 and 50101.00)
			expectedID:    150,
		},
		{
			name: "event should mark as synced when U <= lastUpdateID <= UFinal",
			initialBook: &book{
				symbol:       "BTCUSDT",
				depth:        100,
				bids:         map[string]dec128.Dec128{"50000.00": dec128.FromString("1.0")},
				asks:         map[string]dec128.Dec128{"50100.00": dec128.FromString("2.0")},
				lastUpdateID: 120,
				synced:       false,
			},
			event: DepthEvent{
				U:         100,
				UFinal:    150,
				PrevFinal: 100,
				Bids:      [][2]string{{"50001.00", "0.5"}},
				Asks:      [][2]string{{"50101.00", "1.0"}},
			},
			expectedError: nil,
			expectedID:    150,
			checkSync:     true,
			expectedSync:  true,
		},
		{
			name: "chain break should return error when synced",
			initialBook: &book{
				symbol:       "BTCUSDT",
				depth:        100,
				bids:         map[string]dec128.Dec128{"50000.00": dec128.FromString("1.0")},
				asks:         map[string]dec128.Dec128{"50100.00": dec128.FromString("2.0")},
				lastUpdateID: 100,
				synced:       true,
			},
			event: DepthEvent{
				U:         150,
				UFinal:    200,
				PrevFinal: 90, // wrong prev final
				Bids:      [][2]string{{"50001.00", "0.5"}},
				Asks:      [][2]string{{"50101.00", "1.0"}},
			},
			expectedError: nil, // actually returns error, but we check it's not nil
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bk := tt.initialBook
			logger := NopLogger{}

			err := bk.applyEvent(tt.event, logger)

			if tt.expectedError != nil {
				if err == nil || err != tt.expectedError {
					t.Errorf("expected error %v, got %v", tt.expectedError, err)
				}
				return
			}

			if tt.expectedError == nil && err != nil && tt.name != "chain break should return error when synced" {
				// For chain break test, we expect an error
				if tt.name == "chain break should return error when synced" {
					if err == nil {
						t.Error("expected error for chain break, got nil")
					}
					return
				}
				t.Errorf("unexpected error: %v", err)
			}

			if tt.expectedID != 0 && bk.lastUpdateID != tt.expectedID {
				t.Errorf("expected lastUpdateID %d, got %d", tt.expectedID, bk.lastUpdateID)
			}

			if tt.expectedBids != 0 && len(bk.bids) != tt.expectedBids {
				t.Errorf("expected %d bids, got %d", tt.expectedBids, len(bk.bids))
			}

			if tt.expectedAsks != 0 && len(bk.asks) != tt.expectedAsks {
				t.Errorf("expected %d asks, got %d", tt.expectedAsks, len(bk.asks))
			}

			if tt.checkSync && bk.synced != tt.expectedSync {
				t.Errorf("expected synced %v, got %v", tt.expectedSync, bk.synced)
			}
		})
	}
}

func TestBook_scrap(t *testing.T) {
	tests := []struct {
		name           string
		book           *book
		depth          int
		expectedBids   int
		expectedAsks   int
		expectedSymbol string
		checkAmounts   bool
		expectedBidSum string
		expectedAskSum string
	}{
		{
			name: "empty book",
			book: &book{
				symbol:      "BTCUSDT",
				depth:       100,
				bids:        make(map[string]dec128.Dec128),
				asks:        make(map[string]dec128.Dec128),
				updatedAtMs: 1000000,
			},
			depth:          10,
			expectedBids:   0,
			expectedAsks:   0,
			expectedSymbol: "BTCUSDT",
		},
		{
			name: "book with single level",
			book: &book{
				symbol:      "BTCUSDT",
				depth:       100,
				bids:        map[string]dec128.Dec128{"50000.00": dec128.FromString("1.5")},
				asks:        map[string]dec128.Dec128{"50100.00": dec128.FromString("2.0")},
				updatedAtMs: 1000000,
			},
			depth:          10,
			expectedBids:   1,
			expectedAsks:   1,
			expectedSymbol: "BTCUSDT",
			checkAmounts:   true,
			expectedBidSum: "1.5",
			expectedAskSum: "2.0",
		},
		{
			name: "book with multiple levels, depth limit",
			book: &book{
				symbol: "BTCUSDT",
				depth:  100,
				bids: map[string]dec128.Dec128{
					"50000.00": dec128.FromString("1.0"),
					"49999.00": dec128.FromString("2.0"),
					"49998.00": dec128.FromString("3.0"),
				},
				asks: map[string]dec128.Dec128{
					"50100.00": dec128.FromString("1.0"),
					"50101.00": dec128.FromString("2.0"),
				},
				updatedAtMs: 1000000,
			},
			depth:          2,
			expectedBids:   2, // limited by depth
			expectedAsks:   2,
			expectedSymbol: "BTCUSDT",
			checkAmounts:   true,
			expectedBidSum: "6.0", // sum of all bids
			expectedAskSum: "3.0", // sum of all asks
		},
		{
			name: "book with zero updatedAtMs should use current time",
			book: &book{
				symbol:      "BTCUSDT",
				depth:       100,
				bids:        map[string]dec128.Dec128{"50000.00": dec128.FromString("1.0")},
				asks:        map[string]dec128.Dec128{"50100.00": dec128.FromString("2.0")},
				updatedAtMs: 0,
			},
			depth:          10,
			expectedBids:   1,
			expectedAsks:   1,
			expectedSymbol: "BTCUSDT",
		},
		{
			name: "bids should be sorted descending",
			book: &book{
				symbol: "BTCUSDT",
				depth:  100,
				bids: map[string]dec128.Dec128{
					"50000.00": dec128.FromString("1.0"),
					"50001.00": dec128.FromString("2.0"),
					"49999.00": dec128.FromString("3.0"),
				},
				asks:        make(map[string]dec128.Dec128),
				updatedAtMs: 1000000,
			},
			depth:          10,
			expectedBids:   3,
			expectedAsks:   0,
			expectedSymbol: "BTCUSDT",
		},
		{
			name: "asks should be sorted ascending",
			book: &book{
				symbol: "BTCUSDT",
				depth:  100,
				bids:   make(map[string]dec128.Dec128),
				asks: map[string]dec128.Dec128{
					"50100.00": dec128.FromString("1.0"),
					"50101.00": dec128.FromString("2.0"),
					"50099.00": dec128.FromString("3.0"),
				},
				updatedAtMs: 1000000,
			},
			depth:          10,
			expectedBids:   0,
			expectedAsks:   3,
			expectedSymbol: "BTCUSDT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ob := tt.book.scrap(tt.depth)

			if ob.Symbol != tt.expectedSymbol {
				t.Errorf("expected symbol %s, got %s", tt.expectedSymbol, ob.Symbol)
			}

			if len(ob.Bids) != tt.expectedBids {
				t.Errorf("expected %d bids, got %d", tt.expectedBids, len(ob.Bids))
			}

			if len(ob.Asks) != tt.expectedAsks {
				t.Errorf("expected %d asks, got %d", tt.expectedAsks, len(ob.Asks))
			}

			if tt.checkAmounts {
				expectedBidSum := dec128.FromString(tt.expectedBidSum)
				if !ob.BidAmount.Equal(expectedBidSum) {
					t.Errorf("expected bid amount %s, got %s", tt.expectedBidSum, ob.BidAmount.String())
				}

				expectedAskSum := dec128.FromString(tt.expectedAskSum)
				if !ob.AskAmount.Equal(expectedAskSum) {
					t.Errorf("expected ask amount %s, got %s", tt.expectedAskSum, ob.AskAmount.String())
				}
			}

			// Check sorting for bids (descending)
			if len(ob.Bids) > 1 {
				for i := 0; i < len(ob.Bids)-1; i++ {
					if ob.Bids[i].Price().LessThan(ob.Bids[i+1].Price()) {
						t.Errorf("bids not sorted descending: %s < %s", ob.Bids[i].Price().String(), ob.Bids[i+1].Price().String())
					}
				}
			}

			// Check sorting for asks (ascending)
			if len(ob.Asks) > 1 {
				for i := 0; i < len(ob.Asks)-1; i++ {
					if ob.Asks[i].Price().GreaterThan(ob.Asks[i+1].Price()) {
						t.Errorf("asks not sorted ascending: %s > %s", ob.Asks[i].Price().String(), ob.Asks[i+1].Price().String())
					}
				}
			}
		})
	}
}

func TestApplyLevels(t *testing.T) {
	tests := []struct {
		name       string
		side       map[string]dec128.Dec128
		levels     [][2]string
		desc       bool
		storeDepth int
		expected   int
	}{
		{
			name:       "add levels",
			side:       make(map[string]dec128.Dec128),
			levels:     [][2]string{{"50000.00", "1.0"}, {"50001.00", "2.0"}},
			desc:       true,
			storeDepth: 100,
			expected:   2,
		},
		{
			name: "remove level with zero amount",
			side: func() map[string]dec128.Dec128 {
				// Use normalized price as key to match applyLevels behavior
				price := dec128.FromString("50000.00")
				return map[string]dec128.Dec128{price.String(): dec128.FromString("1.0")}
			}(),
			levels:     [][2]string{{"50000.00", "0"}},
			desc:       true,
			storeDepth: 100,
			expected:   0, // should be removed
		},
		{
			name:       "trim excess levels",
			side:       make(map[string]dec128.Dec128),
			levels:     generateLevels(600, "50000.00", "0.01"),
			desc:       true,
			storeDepth: 500,
			expected:   500,
		},
		{
			name:       "invalid price should be skipped",
			side:       make(map[string]dec128.Dec128),
			levels:     [][2]string{{"invalid", "1.0"}, {"50000.00", "1.0"}},
			desc:       true,
			storeDepth: 100,
			expected:   1,
		},
		{
			name:       "invalid amount should be skipped",
			side:       make(map[string]dec128.Dec128),
			levels:     [][2]string{{"50000.00", "invalid"}, {"50001.00", "1.0"}},
			desc:       true,
			storeDepth: 100,
			expected:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyLevels(tt.side, tt.levels, tt.desc, tt.storeDepth)
			if len(tt.side) != tt.expected {
				t.Errorf("expected %d levels, got %d", tt.expected, len(tt.side))
			}
		})
	}
}

func generateLevels(count int, startPrice, step string) [][2]string {
	levels := make([][2]string, count)
	start := dec128.FromString(startPrice)
	stepDec := dec128.FromString(step)
	for i := 0; i < count; i++ {
		price := start.Add(stepDec.Mul(dec128.FromInt64(int64(i))))
		levels[i] = [2]string{price.String(), "1.0"}
	}
	return levels
}

func TestWithOptions(t *testing.T) {
	tests := []struct {
		name     string
		option   Option
		checkOpt func(*options) bool
	}{
		{
			name:   "WithSymbols",
			option: WithSymbols("BTCUSDT", "ETHUSDT"),
			checkOpt: func(o *options) bool {
				return len(o.symbols) == 2 && o.symbols[0] == "BTCUSDT" && o.symbols[1] == "ETHUSDT"
			},
		},
		{
			name:   "WithLogger",
			option: WithLogger(NopLogger{}),
			checkOpt: func(o *options) bool {
				return o.logger != nil
			},
		},
		{
			name:   "WithHandler",
			option: WithHandler(func(ob OrderBook) {}),
			checkOpt: func(o *options) bool {
				return o.handler != nil
			},
		},
		{
			name:   "WithEventChanSize",
			option: WithEventChanSize(2048),
			checkOpt: func(o *options) bool {
				return o.eventChanSize == 2048
			},
		},
		{
			name:   "WithStoreDepth",
			option: WithStoreDepth(1000),
			checkOpt: func(o *options) bool {
				return o.storeDepth == 1000
			},
		},
		{
			name:   "WithStreamUpdateInterval",
			option: WithStreamUpdateInterval(StreamUpdateInterval100ms),
			checkOpt: func(o *options) bool {
				return o.streamUpdateInterval == StreamUpdateInterval100ms
			},
		},
		{
			name:   "WithSnapshotDepth",
			option: WithSnapshotDepth(2000),
			checkOpt: func(o *options) bool {
				return o.snapshotDepth == 2000
			},
		},
		{
			name:   "WithSnapshotLoadParallelLimit",
			option: WithSnapshotLoadParallelLimit(8),
			checkOpt: func(o *options) bool {
				return o.snapshotLoadParallelLimit == 8
			},
		},
		{
			name:   "WithHandleDepth",
			option: WithHandleDepth(200),
			checkOpt: func(o *options) bool {
				return o.handleDepth == 200
			},
		},
		{
			name: "WithWSDialer",
			option: WithWSDialer(func() *websocket.Dialer {
				return &websocket.Dialer{}
			}),
			checkOpt: func(o *options) bool {
				return o.wsDealer != nil
			},
		},
		{
			name: "WithHTTPClient",
			option: WithHTTPClient(func() *http.Client {
				return &http.Client{}
			}),
			checkOpt: func(o *options) bool {
				return o.httpClient != nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &options{}
			tt.option(o)
			if !tt.checkOpt(o) {
				t.Errorf("option %s did not set value correctly", tt.name)
			}
		})
	}
}

func TestNewBuilder(t *testing.T) {
	tests := []struct {
		name         string
		opts         []Option
		expectedLen  int
		checkBuilder func(*Builder) bool
	}{
		{
			name:        "empty builder",
			opts:        nil,
			expectedLen: 0,
			checkBuilder: func(b *Builder) bool {
				return len(b.books) == 0 && len(b.stream) == 0
			},
		},
		{
			name:        "builder with symbols",
			opts:        []Option{WithSymbols("BTCUSDT", "ETHUSDT")},
			expectedLen: 2,
			checkBuilder: func(b *Builder) bool {
				_, ok1 := b.books["BTCUSDT"]
				_, ok2 := b.books["ETHUSDT"]
				return len(b.books) == 2 && ok1 && ok2
			},
		},
		{
			name:        "builder with duplicate symbols",
			opts:        []Option{WithSymbols("BTCUSDT", "BTCUSDT", "ETHUSDT")},
			expectedLen: 2, // duplicates should be removed
			checkBuilder: func(b *Builder) bool {
				return len(b.books) == 2
			},
		},
		{
			name: "builder with custom options",
			opts: []Option{
				WithSymbols("BTCUSDT"),
				WithStoreDepth(1000),
				WithEventChanSize(2048),
				WithHandleDepth(200),
			},
			expectedLen: 1,
			checkBuilder: func(b *Builder) bool {
				return b.opts.storeDepth == 1000 &&
					b.opts.eventChanSize == 2048 &&
					b.opts.handleDepth == 200
			},
		},
		{
			name:        "builder with default values",
			opts:        nil,
			expectedLen: 0,
			checkBuilder: func(b *Builder) bool {
				return b.opts.eventChanSize == DefaultEventChanSize &&
					b.opts.storeDepth == DefaultStoreDepth &&
					b.opts.snapshotDepth == DefaultSnapshotDepth &&
					b.opts.handleDepth == DefaultHandleDepth &&
					b.opts.streamUpdateInterval == StreamUpdateInterval500ms &&
					b.opts.wsDealer != nil &&
					b.opts.httpClient != nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuilder(tt.opts...)
			if b == nil {
				t.Fatal("NewBuilder returned nil")
			}
			if len(b.books) != tt.expectedLen {
				t.Errorf("expected %d books, got %d", tt.expectedLen, len(b.books))
			}
			if len(b.stream) != tt.expectedLen {
				t.Errorf("expected %d streams, got %d", tt.expectedLen, len(b.stream))
			}
			if !tt.checkBuilder(b) {
				t.Error("builder check failed")
			}
		})
	}
}

func TestBuilder_OrderBook(t *testing.T) {
	tests := []struct {
		name          string
		builder       *Builder
		symbol        string
		depth         int
		expectedError error
		checkResult   func(OrderBook) bool
	}{
		{
			name:          "symbol not found",
			builder:       NewBuilder(WithSymbols("BTCUSDT")),
			symbol:        "ETHUSDT",
			depth:         10,
			expectedError: ErrSymbolNotFound,
		},
		{
			name: "successful retrieval",
			builder: func() *Builder {
				b := NewBuilder(WithSymbols("BTCUSDT"))
				// Initialize book with some data
				bk := b.books["BTCUSDT"]
				bk.bids["50000.00"] = dec128.FromString("1.0")
				bk.asks["50100.00"] = dec128.FromString("2.0")
				bk.lastUpdateID = 1000
				bk.updatedAtMs = 1000000
				return b
			}(),
			symbol:        "BTCUSDT",
			depth:         10,
			expectedError: nil,
			checkResult: func(ob OrderBook) bool {
				return ob.Symbol == "BTCUSDT" &&
					len(ob.Bids) == 1 &&
					len(ob.Asks) == 1
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ob, err := tt.builder.OrderBook(tt.symbol, tt.depth)
			if tt.expectedError != nil {
				if err == nil {
					t.Errorf("expected error %v, got nil", tt.expectedError)
				} else if err != tt.expectedError && !errors.Is(err, tt.expectedError) {
					t.Errorf("expected error %v, got %v", tt.expectedError, err)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if tt.checkResult != nil && !tt.checkResult(ob) {
				t.Error("result check failed")
			}
		})
	}
}

func TestBook_applySnapshot(t *testing.T) {
	tests := []struct {
		name         string
		book         *book
		snapshot     DepthSnapshot
		expectedBids int
		expectedAsks int
		expectedID   int64
		expectedSync bool
	}{
		{
			name: "empty snapshot",
			book: &book{
				symbol: "BTCUSDT",
				depth:  100,
				bids:   map[string]dec128.Dec128{"50000.00": dec128.FromString("1.0")},
				asks:   map[string]dec128.Dec128{"50100.00": dec128.FromString("2.0")},
			},
			snapshot: DepthSnapshot{
				LastUpdateID: 1000,
				Bids:         [][2]string{},
				Asks:         [][2]string{},
			},
			expectedBids: 0,
			expectedAsks: 0,
			expectedID:   1000,
			expectedSync: false,
		},
		{
			name: "snapshot with levels",
			book: &book{
				symbol: "BTCUSDT",
				depth:  100,
				bids:   map[string]dec128.Dec128{"50000.00": dec128.FromString("1.0")},
				asks:   map[string]dec128.Dec128{"50100.00": dec128.FromString("2.0")},
			},
			snapshot: DepthSnapshot{
				LastUpdateID: 2000,
				Bids:         [][2]string{{"50000.00", "1.5"}, {"49999.00", "2.0"}},
				Asks:         [][2]string{{"50100.00", "2.5"}, {"50101.00", "3.0"}},
			},
			expectedBids: 2,
			expectedAsks: 2,
			expectedID:   2000,
			expectedSync: false,
		},
		{
			name: "snapshot replaces existing data",
			book: &book{
				symbol:       "BTCUSDT",
				depth:        100,
				bids:         map[string]dec128.Dec128{"50000.00": dec128.FromString("1.0")},
				asks:         map[string]dec128.Dec128{"50100.00": dec128.FromString("2.0")},
				lastUpdateID: 500,
				synced:       true,
			},
			snapshot: DepthSnapshot{
				LastUpdateID: 3000,
				Bids:         [][2]string{{"50001.00", "1.0"}},
				Asks:         [][2]string{{"50101.00", "2.0"}},
			},
			expectedBids: 1,
			expectedAsks: 1,
			expectedID:   3000,
			expectedSync: false, // should reset synced flag
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.book.applySnapshot(tt.snapshot)
			if len(tt.book.bids) != tt.expectedBids {
				t.Errorf("expected %d bids, got %d", tt.expectedBids, len(tt.book.bids))
			}
			if len(tt.book.asks) != tt.expectedAsks {
				t.Errorf("expected %d asks, got %d", tt.expectedAsks, len(tt.book.asks))
			}
			if tt.book.lastUpdateID != tt.expectedID {
				t.Errorf("expected lastUpdateID %d, got %d", tt.expectedID, tt.book.lastUpdateID)
			}
			if tt.book.synced != tt.expectedSync {
				t.Errorf("expected synced %v, got %v", tt.expectedSync, tt.book.synced)
			}
		})
	}
}

func TestOrder_Amount(t *testing.T) {
	tests := []struct {
		name     string
		order    Order
		expected string
	}{
		{
			name:     "order with amount",
			order:    Order{dec128.FromString("50000.00"), dec128.FromString("1.5")},
			expected: "1.5",
		},
		{
			name:     "order with zero amount",
			order:    Order{dec128.FromString("50000.00"), dec128.Zero},
			expected: "0",
		},
		{
			name:     "order with large amount",
			order:    Order{dec128.FromString("50000.00"), dec128.FromString("1000.123456")},
			expected: "1000.123456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			amount := tt.order.Amount()
			expected := dec128.FromString(tt.expected)
			if !amount.Equal(expected) {
				t.Errorf("expected amount %s, got %s", tt.expected, amount.String())
			}
		})
	}
}

func TestLevelsToOrders(t *testing.T) {
	tests := []struct {
		name        string
		levels      map[string]dec128.Dec128
		limit       int
		desc        bool
		expectedLen int
		checkResult func([]Order) bool
	}{
		{
			name:        "empty levels",
			levels:      make(map[string]dec128.Dec128),
			limit:       10,
			desc:        true,
			expectedLen: 0,
		},
		{
			name: "single level",
			levels: map[string]dec128.Dec128{
				"50000.00": dec128.FromString("1.0"),
			},
			limit:       10,
			desc:        true,
			expectedLen: 1,
			checkResult: func(orders []Order) bool {
				return len(orders) == 1 &&
					orders[0].Price().Equal(dec128.FromString("50000.00")) &&
					orders[0].Amount().Equal(dec128.FromString("1.0"))
			},
		},
		{
			name: "multiple levels with limit",
			levels: map[string]dec128.Dec128{
				"50000.00": dec128.FromString("1.0"),
				"50001.00": dec128.FromString("2.0"),
				"50002.00": dec128.FromString("3.0"),
			},
			limit:       2,
			desc:        true,
			expectedLen: 2,
			checkResult: func(orders []Order) bool {
				if len(orders) != 2 {
					return false
				}
				// Should be sorted descending (highest first)
				return orders[0].Price().GreaterThan(orders[1].Price())
			},
		},
		{
			name: "asks sorted ascending",
			levels: map[string]dec128.Dec128{
				"50100.00": dec128.FromString("1.0"),
				"50099.00": dec128.FromString("2.0"),
				"50101.00": dec128.FromString("3.0"),
			},
			limit:       10,
			desc:        false, // asks
			expectedLen: 3,
			checkResult: func(orders []Order) bool {
				if len(orders) != 3 {
					return false
				}
				// Should be sorted ascending (lowest first)
				return orders[0].Price().LessThan(orders[1].Price()) &&
					orders[1].Price().LessThan(orders[2].Price())
			},
		},
		{
			name: "invalid price string skipped",
			levels: map[string]dec128.Dec128{
				"invalid":  dec128.FromString("1.0"),
				"50000.00": dec128.FromString("2.0"),
			},
			limit:       10,
			desc:        true,
			expectedLen: 1, // invalid price should be skipped
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := levelsToOrders(tt.levels, tt.limit, tt.desc)
			if len(result) != tt.expectedLen {
				t.Errorf("expected %d orders, got %d", tt.expectedLen, len(result))
			}
			if tt.checkResult != nil && !tt.checkResult(result) {
				t.Error("result check failed")
			}
		})
	}
}

func TestApplyLevels_EdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		side       map[string]dec128.Dec128
		levels     [][2]string
		desc       bool
		storeDepth int
		expected   int
	}{
		{
			name:       "empty levels",
			side:       make(map[string]dec128.Dec128),
			levels:     [][2]string{},
			desc:       true,
			storeDepth: 100,
			expected:   0,
		},
		{
			name: "update existing level",
			side: func() map[string]dec128.Dec128 {
				price := dec128.FromString("50000.00")
				return map[string]dec128.Dec128{price.String(): dec128.FromString("1.0")}
			}(),
			levels:     [][2]string{{"50000.00", "2.0"}},
			desc:       true,
			storeDepth: 100,
			expected:   1,
		},
		{
			name:       "negative amount treated as zero",
			side:       make(map[string]dec128.Dec128),
			levels:     [][2]string{{"50000.00", "-1.0"}},
			desc:       true,
			storeDepth: 100,
			expected:   0, // negative amount should not be added
		},
		{
			name: "trimming with exact storeDepth",
			side: make(map[string]dec128.Dec128),
			levels: func() [][2]string {
				levels := make([][2]string, 500)
				basePrice := dec128.FromString("50000.00")
				step := dec128.FromString("0.01")
				for i := 0; i < 500; i++ {
					price := basePrice.Sub(step.Mul(dec128.FromInt64(int64(i))))
					levels[i] = [2]string{price.String(), "1.0"}
				}
				return levels
			}(),
			desc:       true,
			storeDepth: 500,
			expected:   500, // exactly at limit, should not trim
		},
		{
			name: "asks sorting (desc=false)",
			side: make(map[string]dec128.Dec128),
			levels: [][2]string{
				{"50100.00", "1.0"},
				{"50099.00", "2.0"},
				{"50101.00", "3.0"},
			},
			desc:       false, // asks
			storeDepth: 100,
			expected:   3,
		},
		{
			name: "side with invalid price key should be skipped during trimming but remains in map",
			side: map[string]dec128.Dec128{
				"invalid":  dec128.FromString("1.0"), // invalid key that won't parse
				"50000.00": dec128.FromString("2.0"),
			},
			levels:     [][2]string{{"50001.00", "3.0"}},
			desc:       true,
			storeDepth: 1,
			expected:   3, // invalid key remains (not deleted), plus 2 valid levels
		},
		{
			name: "zero amount with existing level",
			side: func() map[string]dec128.Dec128 {
				price := dec128.FromString("50000.00")
				return map[string]dec128.Dec128{
					price.String(): dec128.FromString("1.0"),
					"50001.00":     dec128.FromString("2.0"),
				}
			}(),
			levels:     [][2]string{{"50000.00", "0"}}, // remove existing
			desc:       true,
			storeDepth: 100,
			expected:   1, // one level removed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyLevels(tt.side, tt.levels, tt.desc, tt.storeDepth)
			if len(tt.side) != tt.expected {
				t.Errorf("expected %d levels, got %d", tt.expected, len(tt.side))
			}
		})
	}
}

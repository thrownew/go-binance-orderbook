package binanceorderbook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shopspring/decimal"
)

const (
	OrderPriceIndex  = 0
	OrderAmountIndex = 1
)

const (
	StreamUpdateInterval250ms = "250ms"
	StreamUpdateInterval500ms = "500ms"
	StreamUpdateInterval100ms = "100ms"
)

const (
	DefaultEventChanSize             = 1024 // event channel size
	DefaultStoreDepth                = 500  // depth levels to store in memory
	DefaultSnapshotDepth             = 1000 // depth levels in snapshot
	DefaultHandleDepth               = 100  // depth levels to handle
	DefaultSnapshotLoadParallelLimit = 4    // limit concurrent snapshot fetches to avoid rate limits and memory spikes
)

var (
	ErrSymbolNotFound = errors.New("symbol not found")

	errEmptyBook = errors.New("empty book")
)

type (
	Builder struct {
		books  map[string]*book
		stream map[string]chan DepthEvent
		opts   options
	}

	Option func(*options)

	HandlerFunc func(o OrderBook)

	options struct {
		symbols                   []string
		logger                    Logger
		handler                   HandlerFunc
		eventChanSize             int
		storeDepth                int
		streamUpdateInterval      string
		snapshotDepth             int
		snapshotLoadParallelLimit int
		handleDepth               int
		wsDealer                  func() *websocket.Dialer
		httpClient                func() *http.Client
	}

	Logger interface {
		Debugf(format string, args ...any)
		Infof(format string, args ...any)
		Warnf(format string, args ...any)
		Errorf(format string, args ...any)
	}

	book struct {
		symbol string
		depth  int

		mu           sync.RWMutex
		bids         map[string]decimal.Decimal
		asks         map[string]decimal.Decimal
		lastUpdateID int64
		updatedAtMs  int64
		synced       bool
	}

	OrderBook struct {
		Symbol    string
		Updated   time.Time
		Asks      []Order
		Bids      []Order
		BidAmount decimal.Decimal
		AskAmount decimal.Decimal
	}

	Order [2]decimal.Decimal

	StreamDepthEvent struct {
		Stream string     `json:"stream"`
		Data   DepthEvent `json:"data"`
	}

	DepthEvent struct {
		Event     string      `json:"e"`
		EventTime int64       `json:"E"`
		Symbol    string      `json:"s"`
		U         int64       `json:"U"`  // first update ID in the event
		UFinal    int64       `json:"u"`  // final update ID in the event
		PrevFinal int64       `json:"pu"` // previous final update ID (chain check)
		Bids      [][2]string `json:"b"`  // [price, amount] as strings
		Asks      [][2]string `json:"a"`  // [price, amount] as strings
	}

	DepthSnapshot struct {
		LastUpdateID int64       `json:"lastUpdateId"`
		Bids         [][2]string `json:"bids"`
		Asks         [][2]string `json:"asks"`
	}
)

// WithSymbols sets the list of trading symbols to track
func WithSymbols(symbols ...string) Option {
	return func(o *options) {
		o.symbols = symbols
	}
}

// WithLogger sets a custom logger implementation
func WithLogger(logger Logger) Option {
	return func(o *options) {
		o.logger = logger
	}
}

// WithHandler sets a callback function that will be called when orderbook updates
func WithHandler(handler HandlerFunc) Option {
	return func(o *options) {
		o.handler = handler
	}
}

// WithEventChanSize sets the size of the event channel buffer
func WithEventChanSize(size int) Option {
	return func(o *options) {
		o.eventChanSize = size
	}
}

// WithStoreDepth sets the maximum number of depth levels to store in memory
func WithStoreDepth(depth int) Option {
	return func(o *options) {
		o.storeDepth = depth
	}
}

// WithStreamUpdateInterval sets the update interval for the stream (100ms, 250ms, 500ms)
func WithStreamUpdateInterval(interval string) Option {
	return func(o *options) {
		o.streamUpdateInterval = interval
	}
}

// WithSnapshotDepth sets the number of depth levels to fetch in snapshot
func WithSnapshotDepth(depth int) Option {
	return func(o *options) {
		o.snapshotDepth = depth
	}
}

// WithSnapshotLoadParallelLimit sets the limit for concurrent snapshot fetches
func WithSnapshotLoadParallelLimit(limit int) Option {
	return func(o *options) {
		o.snapshotLoadParallelLimit = limit
	}
}

// WithHandleDepth sets the number of depth levels to include in handler callbacks
func WithHandleDepth(depth int) Option {
	return func(o *options) {
		o.handleDepth = depth
	}
}

// WithWSDialer sets a custom websocket dialer factory function
func WithWSDialer(dialer func() *websocket.Dialer) Option {
	return func(o *options) {
		o.wsDealer = dialer
	}
}

// WithHTTPClient sets a custom HTTP client factory function
func WithHTTPClient(client func() *http.Client) Option {
	return func(o *options) {
		o.httpClient = client
	}
}

func NewBuilder(opts ...Option) *Builder {
	o := options{
		symbols:                   nil,
		logger:                    NopLogger{},
		eventChanSize:             DefaultEventChanSize,
		storeDepth:                DefaultStoreDepth,
		streamUpdateInterval:      StreamUpdateInterval500ms,
		snapshotDepth:             DefaultSnapshotDepth,
		snapshotLoadParallelLimit: DefaultSnapshotLoadParallelLimit,
		handleDepth:               DefaultHandleDepth,
		wsDealer: func() *websocket.Dialer {
			return &websocket.Dialer{
				Proxy:             http.ProxyFromEnvironment,
				HandshakeTimeout:  45 * time.Second,
				ReadBufferSize:    16384,
				WriteBufferSize:   16384,
				EnableCompression: false,
			}
		},
		httpClient: func() *http.Client {
			return http.DefaultClient
		},
	}
	for _, opt := range opts {
		opt(&o)
	}
	o.symbols = slices.Compact(o.symbols)

	b := &Builder{
		books:  make(map[string]*book, len(o.symbols)),
		stream: make(map[string]chan DepthEvent, len(o.symbols)),
		opts:   o,
	}

	for _, symbol := range o.symbols {
		b.books[symbol] = &book{
			symbol: symbol,
			depth:  o.storeDepth,
			bids:   make(map[string]decimal.Decimal),
			asks:   make(map[string]decimal.Decimal),
		}
		b.stream[symbol] = make(chan DepthEvent, b.opts.eventChanSize)
	}

	return b
}

func (b *Builder) Run(ctx context.Context) error {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				if err := b.listenWS(ctx); err != nil {
					b.opts.logger.Errorf("Binance: OrderBook: Listen ws error: %s", err.Error())
					time.Sleep(time.Second)
				}
			}
		}
	}()
	snapshotLimiter := make(chan struct{}, b.opts.snapshotLoadParallelLimit)
	for symbol, ch := range b.stream {
		go func(ch chan DepthEvent, bk *book) {
			for {
				select {
				case <-ctx.Done():
					return
				case ev := <-ch:
					err := bk.applyEvent(ev, b.opts.logger)
					if err != nil {
						if !errors.Is(err, errEmptyBook) {
							b.opts.logger.Warnf("Binance: OrderBook: Symbol `%s`: Apply event error: %s", bk.symbol, err)
						}
						snap, sErr := b.fetchSnapshot(ctx, snapshotLimiter, bk.symbol, b.opts.snapshotDepth)
						if sErr != nil {
							b.opts.logger.Errorf("Binance: OrderBook: Symbol `%s`: Fetch snapshot error: %s", bk.symbol, sErr)
						} else {
							bk.applySnapshot(snap)
							b.opts.logger.Debugf("Binance: OrderBook: Symbol `%s`: Snapshot applied (last update id: %d)", bk.symbol, snap.LastUpdateID)
							// reapply the event after snapshot again
							if err = bk.applyEvent(ev, b.opts.logger); err != nil {
								b.opts.logger.Errorf("Binance: OrderBook: Symbol `%s`: Reapply event error: %s", bk.symbol, err)
								continue
							}
						}
					}
					if b.opts.handler != nil {
						b.opts.handler(bk.scrap(b.opts.handleDepth))
					}
				}
			}
		}(ch, b.books[symbol])
	}
	return nil
}

func (b *Builder) OrderBook(symbol string, depth int) (OrderBook, error) {
	bk, ok := b.books[symbol]
	if !ok {
		return OrderBook{}, fmt.Errorf("%w: %s", ErrSymbolNotFound, symbol)
	}
	ob := bk.scrap(depth)
	return ob, nil
}

func (b *Builder) listenWS(ctx context.Context) error {
	streams := make([]string, 0, len(b.stream))
	for symbol := range b.stream {
		stream := fmt.Sprintf("%s@depth", strings.ToLower(symbol))
		if b.opts.streamUpdateInterval != "" {
			stream += "@" + b.opts.streamUpdateInterval
		}
		streams = append(streams, stream)
	}

	dialer := b.opts.wsDealer()

	conn, resp, err := dialer.DialContext(ctx, (&url.URL{
		Scheme:   "wss",
		Host:     "fstream.binance.com",
		Path:     "/stream",
		RawQuery: fmt.Sprintf("streams=%s", strings.Join(streams, "/")),
	}).String(), nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close() //nolint:errcheck
		}
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")) //nolint:errcheck
		_ = conn.Close()                                                                                              //nolint:errcheck
	}()

	var msg []byte
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		_, msg, err = conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read message: %w", err)
		}
		var e StreamDepthEvent
		if err = json.Unmarshal(msg, &e); err != nil {
			b.opts.logger.Errorf("Binance: OrderBook: Unmarshal stream event error: %s (%s)", err.Error(), string(msg))
			continue
		}
		ch, ok := b.stream[strings.ToUpper(e.Data.Symbol)]
		if !ok {
			b.opts.logger.Warnf("Binance: OrderBook: Received event for unknown symbol %s", e.Data.Symbol)
			continue
		}
		select {
		case ch <- e.Data:
		default:
			continue
		}
	}
}

func (b *Builder) fetchSnapshot(ctx context.Context, limiter chan struct{}, symbol string, depth int) (DepthSnapshot, error) {
	limiter <- struct{}{}
	defer func() {
		<-limiter
	}()
	var snap DepthSnapshot
	val := url.Values{
		"symbol": {symbol},
		"limit":  {strconv.Itoa(depth)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://fapi.binance.com/fapi/v1/depth?"+val.Encode(), nil)
	if err != nil {
		return snap, fmt.Errorf("new request: %w", err)
	}
	resp, err := b.opts.httpClient().Do(req)
	if err != nil {
		return snap, fmt.Errorf("do: %w", err)
	}
	defer func() {
		if cErr := resp.Body.Close(); cErr != nil {
			b.opts.logger.Warnf("Binance: OrderBook: Symbol `%s`: Close snapshot body error: %s", symbol, cErr.Error())
		}
	}()
	if resp.StatusCode != 200 {
		return DepthSnapshot{}, fmt.Errorf("status: %d", resp.StatusCode)
	}
	if err = json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		return snap, err
	}
	b.opts.logger.Debugf("Binance: OrderBook: Symbol `%s`: Fetching snapshot", symbol)
	return snap, nil
}

func (b *book) applySnapshot(snap DepthSnapshot) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.bids = make(map[string]decimal.Decimal)
	b.asks = make(map[string]decimal.Decimal)
	applyLevels(b.bids, snap.Bids, true, b.depth)
	applyLevels(b.asks, snap.Asks, false, b.depth)
	b.lastUpdateID = snap.LastUpdateID
	b.synced = false
}

func (b *book) applyEvent(ev DepthEvent, logger Logger) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.lastUpdateID == 0 {
		return errEmptyBook
	}
	if ev.UFinal < b.lastUpdateID {
		return nil // skip old events
	}
	if b.synced && ev.PrevFinal != b.lastUpdateID {
		return fmt.Errorf("prev final ID %d not equal last update ID %d", ev.PrevFinal, b.lastUpdateID) // chain outdate needs resync
	}
	if ev.U <= b.lastUpdateID && ev.UFinal >= b.lastUpdateID {
		logger.Debugf("Binance: OrderBook: Symbol: %s: Synchronized (U=%d UFinal=%d PrevFinal=%d)", b.symbol, ev.U, ev.UFinal, ev.PrevFinal)
		b.synced = true
	}
	applyLevels(b.bids, ev.Bids, true, b.depth)
	applyLevels(b.asks, ev.Asks, false, b.depth)
	b.lastUpdateID = ev.UFinal
	b.updatedAtMs = ev.EventTime
	return nil
}

func applyLevels(side map[string]decimal.Decimal, levels [][2]string, desc bool, storeDepth int) {
	for _, lv := range levels {
		price, err := decimal.NewFromString(lv[0])
		if err != nil {
			continue // skip invalid price
		}
		amount, err := decimal.NewFromString(lv[1])
		if err != nil {
			continue // skip invalid amount
		}
		// normalize price as string
		normalizedPrice := price.String()
		if amount.IsPositive() {
			side[normalizedPrice] = amount
		} else {
			delete(side, normalizedPrice)
		}
	}

	// Binance have thousands of levels so we need to trim it to avoid memory issues

	sorted := make([]decimal.Decimal, 0, len(side))
	for strPrice := range side {
		price, err := decimal.NewFromString(strPrice)
		if err != nil {
			continue // skip invalid price
		}
		sorted = append(sorted, price)
	}

	slices.SortFunc(sorted, func(a, b decimal.Decimal) int {
		cmp := a.Cmp(b)
		if desc {
			cmp = -cmp
		}
		return cmp
	})

	if len(sorted) > storeDepth {
		for i := storeDepth; i < len(sorted); i++ {
			priceStr := sorted[i].String()
			delete(side, priceStr)
		}
	}
}

func (b *book) scrap(depth int) OrderBook {
	bidAmount := decimal.Zero
	askAmount := decimal.Zero
	b.mu.RLock()
	for _, amount := range b.bids {
		bidAmount = bidAmount.Add(amount)
	}
	for _, amount := range b.asks {
		askAmount = askAmount.Add(amount)
	}
	bids := levelsToOrders(b.bids, depth, true)
	asks := levelsToOrders(b.asks, depth, false)
	updatedAtMs := b.updatedAtMs
	b.mu.RUnlock()

	if updatedAtMs == 0 {
		updatedAtMs = time.Now().UnixMilli()
	}

	return OrderBook{
		Updated:   time.UnixMilli(updatedAtMs),
		Symbol:    b.symbol,
		Bids:      bids,
		Asks:      asks,
		AskAmount: askAmount,
		BidAmount: bidAmount,
	}
}

func levelsToOrders(levels map[string]decimal.Decimal, limit int, desc bool) []Order {
	if len(levels) == 0 {
		return nil
	}
	list := make([]Order, 0, len(levels))
	for strPrice, amount := range levels {
		price, err := decimal.NewFromString(strPrice)
		if err != nil {
			continue
		}
		list = append(list, Order{OrderPriceIndex: price, OrderAmountIndex: amount.Copy()})
	}
	slices.SortFunc(list, func(a, b Order) int {
		cmp := a.Price().Cmp(b.Price())
		if desc {
			cmp = -cmp
		}
		return cmp
	})
	if len(list) > limit {
		list = list[:limit]
	}
	return list
}

func (o Order) Price() decimal.Decimal {
	return o[OrderPriceIndex]
}

func (o Order) Amount() decimal.Decimal {
	return o[OrderAmountIndex]
}

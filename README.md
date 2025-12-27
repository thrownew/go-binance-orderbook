[![Release](https://img.shields.io/github/release/thrownew/go-binance-orderbook.svg)](https://github.com/thrownew/go-binance-orderbook/releases/latest)
[![License](https://img.shields.io/github/license/thrownew/go-binance-orderbook.svg)](https://raw.githubusercontent.com/thrownew/go-binance-orderbook/master/LICENSE)
[![Godocs](https://img.shields.io/badge/godoc-reference-blue.svg)](https://godoc.org/github.com/thrownew/go-binance-orderbook)

[![Build Status](https://github.com/thrownew/go-binance-orderbook/workflows/CI/badge.svg)](https://github.com/thrownew/go-binance-orderbook/actions)
[![codecov](https://codecov.io/gh/thrownew/go-binance-orderbook/branch/master/graph/badge.svg)](https://codecov.io/gh/thrownew/go-binance-orderbook)
[![Go Report Card](https://goreportcard.com/badge/github.com/thrownew/go-binance-orderbook)](https://goreportcard.com/report/github.com/thrownew/go-binance-orderbook)

# go-binance-orderbook

Binance OrderBook based on [USDⓈ-M Futures](https://developers.binance.com/docs/derivatives/usds-margined-futures/general-info)

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
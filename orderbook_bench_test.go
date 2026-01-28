package binanceorderbook

import (
	"testing"

	"github.com/jokruger/dec128"
)

// generateTestBook creates a book with realistic data
func generateTestBook(symbol string, depth int, numLevels int) *book {
	bk := &book{
		symbol:       symbol,
		depth:        depth,
		bids:         make(map[string]dec128.Dec128, numLevels),
		asks:         make(map[string]dec128.Dec128, numLevels),
		lastUpdateID: 1000000,
		updatedAtMs:  1000000000000,
		synced:       true,
	}

	// Generate realistic price levels
	basePrice := dec128.FromString("50000.00")
	step := dec128.FromString("0.01")

	for i := 0; i < numLevels; i++ {
		bidPrice := basePrice.Sub(step.Mul(dec128.FromInt64(int64(i))))
		askPrice := basePrice.Add(step.Mul(dec128.FromInt64(int64(i))))

		bidAmount := dec128.FromFloat64(0.1 + float64(i)*0.01)
		askAmount := dec128.FromFloat64(0.1 + float64(i)*0.01)

		bk.bids[bidPrice.String()] = bidAmount
		bk.asks[askPrice.String()] = askAmount
	}

	return bk
}

// generateTestEvent creates a realistic depth event with updates at various depth levels
func generateTestEvent(symbol string, updateID int64, numUpdates int) DepthEvent {
	basePrice := dec128.FromString("50000.00")
	step := dec128.FromString("0.01")

	bids := make([][2]string, 0, numUpdates)
	asks := make([][2]string, 0, numUpdates)

	// Generate updates at different depth levels, not just L1
	// Mix of: top levels (L1-L5), middle levels, and deeper levels
	for i := 0; i < numUpdates; i++ {
		var levelIndex int

		// Distribute updates across different depth levels
		switch i % 4 {
		case 0:
			// Top levels (L1-L10): 25% of updates
			levelIndex = i / 4
		case 1:
			// Middle levels (L20-L100): 25% of updates
			levelIndex = 20 + (i/4)*5
		case 2:
			// Deeper levels (L100-L300): 25% of updates
			levelIndex = 100 + (i/4)*10
		case 3:
			// Random deeper levels (L50-L400): 25% of updates
			levelIndex = 50 + (i/4)*15
		}

		bidPrice := basePrice.Sub(step.Mul(dec128.FromInt64(int64(levelIndex))))
		askPrice := basePrice.Add(step.Mul(dec128.FromInt64(int64(levelIndex))))

		// Vary amounts - some updates, some removals (zero amount)
		amount := dec128.FromFloat64(0.1 + float64(i)*0.01)
		if i%7 == 0 {
			// Occasionally remove a level (zero amount)
			amount = dec128.Zero
		}

		bids = append(bids, [2]string{bidPrice.String(), amount.String()})
		asks = append(asks, [2]string{askPrice.String(), amount.String()})
	}

	return DepthEvent{
		Event:     "depthUpdate",
		EventTime: 1000000000000,
		Symbol:    symbol,
		U:         updateID,
		UFinal:    updateID + 10,
		PrevFinal: updateID - 10,
		Bids:      bids,
		Asks:      asks,
	}
}

// generateTestEventTopLevels creates events that only update top levels (L1-L10)
func generateTestEventTopLevels(symbol string, updateID int64, numUpdates int) DepthEvent {
	basePrice := dec128.FromString("50000.00")
	step := dec128.FromString("0.01")

	bids := make([][2]string, numUpdates)
	asks := make([][2]string, numUpdates)

	for i := 0; i < numUpdates; i++ {
		levelIndex := i % 10 // Only top 10 levels
		bidPrice := basePrice.Sub(step.Mul(dec128.FromInt64(int64(levelIndex))))
		askPrice := basePrice.Add(step.Mul(dec128.FromInt64(int64(levelIndex))))

		bids[i] = [2]string{bidPrice.String(), dec128.FromFloat64(0.1 + float64(i)*0.01).String()}
		asks[i] = [2]string{askPrice.String(), dec128.FromFloat64(0.1 + float64(i)*0.01).String()}
	}

	return DepthEvent{
		Event:     "depthUpdate",
		EventTime: 1000000000000,
		Symbol:    symbol,
		U:         updateID,
		UFinal:    updateID + 10,
		PrevFinal: updateID - 10,
		Bids:      bids,
		Asks:      asks,
	}
}

// generateTestEventDeepLevels creates events that update deeper levels (L50+)
func generateTestEventDeepLevels(symbol string, updateID int64, numUpdates int) DepthEvent {
	basePrice := dec128.FromString("50000.00")
	step := dec128.FromString("0.01")

	bids := make([][2]string, numUpdates)
	asks := make([][2]string, numUpdates)

	for i := 0; i < numUpdates; i++ {
		levelIndex := 50 + i*5 // Start from level 50, skip by 5
		bidPrice := basePrice.Sub(step.Mul(dec128.FromInt64(int64(levelIndex))))
		askPrice := basePrice.Add(step.Mul(dec128.FromInt64(int64(levelIndex))))

		bids[i] = [2]string{bidPrice.String(), dec128.FromFloat64(0.1 + float64(i)*0.01).String()}
		asks[i] = [2]string{askPrice.String(), dec128.FromFloat64(0.1 + float64(i)*0.01).String()}
	}

	return DepthEvent{
		Event:     "depthUpdate",
		EventTime: 1000000000000,
		Symbol:    symbol,
		U:         updateID,
		UFinal:    updateID + 10,
		PrevFinal: updateID - 10,
		Bids:      bids,
		Asks:      asks,
	}
}

// generateTestEventMixed creates events with mixed update patterns
func generateTestEventMixed(symbol string, updateID int64, numUpdates int) DepthEvent {
	basePrice := dec128.FromString("50000.00")
	step := dec128.FromString("0.01")

	bids := make([][2]string, 0, numUpdates)
	asks := make([][2]string, 0, numUpdates)

	for i := 0; i < numUpdates; i++ {
		var levelIndex int
		var amount dec128.Dec128

		switch i % 5 {
		case 0:
			// L1-L5: frequent top level updates
			levelIndex = i % 5
			amount = dec128.FromFloat64(0.5 + float64(i)*0.1)
		case 1:
			// L10-L50: middle level updates
			levelIndex = 10 + (i%5)*10
			amount = dec128.FromFloat64(0.2 + float64(i)*0.05)
		case 2:
			// L100-L200: deeper level updates
			levelIndex = 100 + (i%5)*25
			amount = dec128.FromFloat64(0.1 + float64(i)*0.02)
		case 3:
			// Remove levels (zero amount) at various depths
			levelIndex = 5 + (i%5)*20
			amount = dec128.Zero
		case 4:
			// Very deep levels L300+
			levelIndex = 300 + (i%5)*30
			amount = dec128.FromFloat64(0.05 + float64(i)*0.01)
		}

		bidPrice := basePrice.Sub(step.Mul(dec128.FromInt64(int64(levelIndex))))
		askPrice := basePrice.Add(step.Mul(dec128.FromInt64(int64(levelIndex))))

		bids = append(bids, [2]string{bidPrice.String(), amount.String()})
		asks = append(asks, [2]string{askPrice.String(), amount.String()})
	}

	return DepthEvent{
		Event:     "depthUpdate",
		EventTime: 1000000000000,
		Symbol:    symbol,
		U:         updateID,
		UFinal:    updateID + 10,
		PrevFinal: updateID - 10,
		Bids:      bids,
		Asks:      asks,
	}
}

// BenchmarkBook_applyEvent_Small - small updates at various depth levels
func BenchmarkBook_applyEvent_Small(b *testing.B) {
	bk := generateTestBook("BTCUSDT", 500, 100)
	event := generateTestEvent("BTCUSDT", 1000010, 10)
	logger := NopLogger{}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = bk.applyEvent(event, logger)
		// Reset event ID for next iteration
		event.UFinal = bk.lastUpdateID + 10
		event.PrevFinal = bk.lastUpdateID
		event.U = bk.lastUpdateID + 1
	}
}

// BenchmarkBook_applyEvent_Medium - medium updates at various depth levels
func BenchmarkBook_applyEvent_Medium(b *testing.B) {
	bk := generateTestBook("BTCUSDT", 500, 500)
	event := generateTestEvent("BTCUSDT", 1000010, 50)
	logger := NopLogger{}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = bk.applyEvent(event, logger)
		// Reset event ID for next iteration
		event.UFinal = bk.lastUpdateID + 10
		event.PrevFinal = bk.lastUpdateID
		event.U = bk.lastUpdateID + 1
	}
}

// BenchmarkBook_applyEvent_Large - large updates at various depth levels
func BenchmarkBook_applyEvent_Large(b *testing.B) {
	bk := generateTestBook("BTCUSDT", 500, 500)
	event := generateTestEvent("BTCUSDT", 1000010, 200)
	logger := NopLogger{}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = bk.applyEvent(event, logger)
		// Reset event ID for next iteration
		event.UFinal = bk.lastUpdateID + 10
		event.PrevFinal = bk.lastUpdateID
		event.U = bk.lastUpdateID + 1
	}
}

// BenchmarkBook_applyEvent_FullDepth - full depth updates
func BenchmarkBook_applyEvent_FullDepth(b *testing.B) {
	bk := generateTestBook("BTCUSDT", 500, 500)
	event := generateTestEvent("BTCUSDT", 1000010, 500)
	logger := NopLogger{}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = bk.applyEvent(event, logger)
		// Reset event ID for next iteration
		event.UFinal = bk.lastUpdateID + 10
		event.PrevFinal = bk.lastUpdateID
		event.U = bk.lastUpdateID + 1
	}
}

// BenchmarkBook_applyEvent_TopLevels - updates only at top levels (L1-L10)
func BenchmarkBook_applyEvent_TopLevels(b *testing.B) {
	bk := generateTestBook("BTCUSDT", 500, 500)
	event := generateTestEventTopLevels("BTCUSDT", 1000010, 20)
	logger := NopLogger{}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = bk.applyEvent(event, logger)
		// Reset event ID for next iteration
		event.UFinal = bk.lastUpdateID + 10
		event.PrevFinal = bk.lastUpdateID
		event.U = bk.lastUpdateID + 1
	}
}

// BenchmarkBook_applyEvent_DeepLevels - updates only at deep levels (L50+)
func BenchmarkBook_applyEvent_DeepLevels(b *testing.B) {
	bk := generateTestBook("BTCUSDT", 500, 500)
	event := generateTestEventDeepLevels("BTCUSDT", 1000010, 50)
	logger := NopLogger{}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = bk.applyEvent(event, logger)
		// Reset event ID for next iteration
		event.UFinal = bk.lastUpdateID + 10
		event.PrevFinal = bk.lastUpdateID
		event.U = bk.lastUpdateID + 1
	}
}

// BenchmarkBook_applyEvent_Mixed - mixed update patterns (top, middle, deep, removals)
func BenchmarkBook_applyEvent_Mixed(b *testing.B) {
	bk := generateTestBook("BTCUSDT", 500, 500)
	event := generateTestEventMixed("BTCUSDT", 1000010, 100)
	logger := NopLogger{}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = bk.applyEvent(event, logger)
		// Reset event ID for next iteration
		event.UFinal = bk.lastUpdateID + 10
		event.PrevFinal = bk.lastUpdateID
		event.U = bk.lastUpdateID + 1
	}
}

func BenchmarkBook_scrap_SmallDepth(b *testing.B) {
	bk := generateTestBook("BTCUSDT", 500, 100)
	depth := 10

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = bk.scrap(depth)
	}
}

func BenchmarkBook_scrap_MediumDepth(b *testing.B) {
	bk := generateTestBook("BTCUSDT", 500, 500)
	depth := 100

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = bk.scrap(depth)
	}
}

func BenchmarkBook_scrap_LargeDepth(b *testing.B) {
	bk := generateTestBook("BTCUSDT", 500, 500)
	depth := 500

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = bk.scrap(depth)
	}
}

func BenchmarkBook_scrap_FullBook(b *testing.B) {
	bk := generateTestBook("BTCUSDT", 500, 500)
	depth := 500

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = bk.scrap(depth)
	}
}

// BenchmarkApplyLevels_Small - small updates at various depth levels
func BenchmarkApplyLevels_Small(b *testing.B) {
	levels := generateTestLevelsVaried(10)
	side := make(map[string]dec128.Dec128, 100)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		applyLevels(side, levels, true, 500)
		// Clear side for next iteration
		for k := range side {
			delete(side, k)
		}
	}
}

// BenchmarkApplyLevels_Medium - medium updates at various depth levels
func BenchmarkApplyLevels_Medium(b *testing.B) {
	levels := generateTestLevelsVaried(50)
	side := make(map[string]dec128.Dec128, 500)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		applyLevels(side, levels, true, 500)
		// Clear side for next iteration
		for k := range side {
			delete(side, k)
		}
	}
}

// BenchmarkApplyLevels_Large - large updates at various depth levels
func BenchmarkApplyLevels_Large(b *testing.B) {
	levels := generateTestLevelsVaried(200)
	side := make(map[string]dec128.Dec128, 500)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		applyLevels(side, levels, true, 500)
		// Clear side for next iteration
		for k := range side {
			delete(side, k)
		}
	}
}

// BenchmarkApplyLevels_TopLevels - updates only at top levels (L1-L10)
func BenchmarkApplyLevels_TopLevels(b *testing.B) {
	levels := generateTestLevelsTopOnly(20)
	side := make(map[string]dec128.Dec128, 500)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		applyLevels(side, levels, true, 500)
		// Clear side for next iteration
		for k := range side {
			delete(side, k)
		}
	}
}

// BenchmarkApplyLevels_DeepLevels - updates only at deep levels (L50+)
func BenchmarkApplyLevels_DeepLevels(b *testing.B) {
	levels := generateTestLevelsDeepOnly(50)
	side := make(map[string]dec128.Dec128, 500)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		applyLevels(side, levels, true, 500)
		// Clear side for next iteration
		for k := range side {
			delete(side, k)
		}
	}
}

// BenchmarkApplyLevels_Mixed - mixed update patterns (top, middle, deep, removals)
func BenchmarkApplyLevels_Mixed(b *testing.B) {
	levels := generateTestLevelsMixed(100)
	side := make(map[string]dec128.Dec128, 500)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		applyLevels(side, levels, true, 500)
		// Clear side for next iteration
		for k := range side {
			delete(side, k)
		}
	}
}

func BenchmarkLevelsToOrders_Small(b *testing.B) {
	levels := make(map[string]dec128.Dec128, 100)
	basePrice := dec128.FromString("50000.00")
	step := dec128.FromString("0.01")

	for i := 0; i < 100; i++ {
		price := basePrice.Sub(step.Mul(dec128.FromInt64(int64(i))))
		levels[price.String()] = dec128.FromFloat64(0.1 + float64(i)*0.01)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = levelsToOrders(levels, 10, true)
	}
}

func BenchmarkLevelsToOrders_Medium(b *testing.B) {
	levels := make(map[string]dec128.Dec128, 500)
	basePrice := dec128.FromString("50000.00")
	step := dec128.FromString("0.01")

	for i := 0; i < 500; i++ {
		price := basePrice.Sub(step.Mul(dec128.FromInt64(int64(i))))
		levels[price.String()] = dec128.FromFloat64(0.1 + float64(i)*0.01)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = levelsToOrders(levels, 100, true)
	}
}

func BenchmarkLevelsToOrders_Large(b *testing.B) {
	levels := make(map[string]dec128.Dec128, 500)
	basePrice := dec128.FromString("50000.00")
	step := dec128.FromString("0.01")

	for i := 0; i < 500; i++ {
		price := basePrice.Sub(step.Mul(dec128.FromInt64(int64(i))))
		levels[price.String()] = dec128.FromFloat64(0.1 + float64(i)*0.01)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = levelsToOrders(levels, 500, true)
	}
}

// generateTestLevelsVaried - generates levels at various depth positions (not just sequential)
func generateTestLevelsVaried(count int) [][2]string {
	levels := make([][2]string, 0, count)
	basePrice := dec128.FromString("50000.00")
	step := dec128.FromString("0.01")

	for i := 0; i < count; i++ {
		var levelIndex int

		// Distribute across different depth levels
		switch i % 4 {
		case 0:
			// Top levels (L1-L10)
			levelIndex = i / 4
		case 1:
			// Middle levels (L20-L100)
			levelIndex = 20 + (i/4)*5
		case 2:
			// Deeper levels (L100-L300)
			levelIndex = 100 + (i/4)*10
		case 3:
			// Random deeper levels (L50-L400)
			levelIndex = 50 + (i/4)*15
		}

		price := basePrice.Sub(step.Mul(dec128.FromInt64(int64(levelIndex))))
		amount := dec128.FromFloat64(0.1 + float64(i)*0.01)
		if i%7 == 0 {
			// Occasionally remove a level
			amount = dec128.Zero
		}
		levels = append(levels, [2]string{price.String(), amount.String()})
	}

	return levels
}

// generateTestLevelsTopOnly - generates levels only at top levels (L1-L10)
func generateTestLevelsTopOnly(count int) [][2]string {
	levels := make([][2]string, count)
	basePrice := dec128.FromString("50000.00")
	step := dec128.FromString("0.01")

	for i := 0; i < count; i++ {
		levelIndex := i % 10 // Only top 10 levels
		price := basePrice.Sub(step.Mul(dec128.FromInt64(int64(levelIndex))))
		levels[i] = [2]string{price.String(), dec128.FromFloat64(0.1 + float64(i)*0.01).String()}
	}

	return levels
}

// generateTestLevelsDeepOnly - generates levels only at deep levels (L50+)
func generateTestLevelsDeepOnly(count int) [][2]string {
	levels := make([][2]string, count)
	basePrice := dec128.FromString("50000.00")
	step := dec128.FromString("0.01")

	for i := 0; i < count; i++ {
		levelIndex := 50 + i*5 // Start from level 50, skip by 5
		price := basePrice.Sub(step.Mul(dec128.FromInt64(int64(levelIndex))))
		levels[i] = [2]string{price.String(), dec128.FromFloat64(0.1 + float64(i)*0.01).String()}
	}

	return levels
}

// generateTestLevelsMixed - generates levels with mixed update patterns
func generateTestLevelsMixed(count int) [][2]string {
	levels := make([][2]string, 0, count)
	basePrice := dec128.FromString("50000.00")
	step := dec128.FromString("0.01")

	for i := 0; i < count; i++ {
		var levelIndex int
		var amount dec128.Dec128

		switch i % 5 {
		case 0:
			// L1-L5: frequent top level updates
			levelIndex = i % 5
			amount = dec128.FromFloat64(0.5 + float64(i)*0.1)
		case 1:
			// L10-L50: middle level updates
			levelIndex = 10 + (i%5)*10
			amount = dec128.FromFloat64(0.2 + float64(i)*0.05)
		case 2:
			// L100-L200: deeper level updates
			levelIndex = 100 + (i%5)*25
			amount = dec128.FromFloat64(0.1 + float64(i)*0.02)
		case 3:
			// Remove levels (zero amount) at various depths
			levelIndex = 5 + (i%5)*20
			amount = dec128.Zero
		case 4:
			// Very deep levels L300+
			levelIndex = 300 + (i%5)*30
			amount = dec128.FromFloat64(0.05 + float64(i)*0.01)
		}

		price := basePrice.Sub(step.Mul(dec128.FromInt64(int64(levelIndex))))
		levels = append(levels, [2]string{price.String(), amount.String()})
	}

	return levels
}

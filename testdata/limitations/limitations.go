// Package limitations collects programs that Sheriff does not currently
// analyse. Each function is valid Go that contains a real hazard the analyzer
// cannot prove, so running Sheriff over this package should report none of them.
// When a limitation is fixed, its function is removed from this file.
package limitations

// ============================================================
// SHARED LIMITATIONS
// ============================================================

type Config struct {
	Host string
}

// 1. Function values holding a function literal are not resolved.
func functionLiteralNil() {
	factory := func() *Config { return nil }
	cfg := factory()
	_ = cfg.Host // Potential nil pointer dereference.
}

// 2. Imported functions are not resolved.
func importedFunctionNil() {
	// Example to be added when testing an imported package:
	// cfg := external.GetConfig()
	// _ = cfg.Host
}

// ============================================================
// NILPTR LIMITATIONS
// ============================================================

// 3. Pointer values through interfaces.
func interfaceNil() {
	var cfg *Config
	var value any = cfg

	actual := value.(*Config)
	_ = actual.Host // Nil pointer dereference.
}

// 4. Pointer values through maps.
func mapNil() {
	configs := map[string]*Config{
		"server": nil,
	}

	cfg := configs["server"]
	_ = cfg.Host // Nil pointer dereference.
}

// 5. Pointer values through channels.
func channelNil() {
	ch := make(chan *Config, 1)
	ch <- nil

	cfg := <-ch
	_ = cfg.Host // Nil pointer dereference.
}

// 6. Pointer values through goroutines.
func goroutineNil() {
	var cfg *Config

	go func() {
		_ = cfg.Host // Nil pointer dereference.
	}()
}

// ============================================================
// DIVZERO LIMITATIONS
// ============================================================

// 7. Package-level variables.
var divisor = 0

func packageVariableDivision() int {
	return 100 / divisor
}

// 8. Values reached through pointers.
func pointerDivision() int {
	zero := 0
	divisor := &zero

	return 100 / *divisor
}

// 9. Function results.
func getDivisor() int {
	return 0
}

func functionResultDivision() int {
	return 100 / getDivisor()
}

// 10. Type parameters.
func genericDivision[T ~int](a, b T) T {
	return a / b
}

func genericDivisionCall() int {
	return genericDivision(100, 0)
}

// ============================================================
// BOUNDS LIMITATIONS
// ============================================================

// 11. Capacity tracking.
func capacityBounds() int {
	values := make([]int, 1, 4)
	values = values[:cap(values)]

	return values[3]
}

// 12. Loop induction values.
func loopBounds() int {
	values := []int{10, 20, 30}

	for i := 0; i <= len(values); i++ {
		if i == len(values) {
			return values[i] // Out of bounds.
		}
	}

	return 0
}

// ============================================================
// ASSERT LIMITATIONS
// ============================================================

type Envelope struct {
	Value any
}

// 13. Dynamic types through struct fields.
func assertionThroughField() string {
	envelope := Envelope{Value: 123}

	return envelope.Value.(string)
}

// 14. Dynamic types through maps.
func assertionThroughMap() string {
	values := map[string]any{
		"name": 123,
	}

	return values["name"].(string)
}

// 15. Dynamic types through channels.
func assertionThroughChannel() string {
	ch := make(chan any, 1)
	ch <- 123

	value := <-ch
	return value.(string)
}

// 16. Dynamic types through type parameters.
func assertionThroughGeneric[T any](value T) string {
	return any(value).(string)
}

func genericAssertionCall() string {
	return assertionThroughGeneric(123)
}

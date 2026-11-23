package limitations

// ============================================================
// SHARED LIMITATIONS
// ============================================================

type Config struct {
	Host string
}

// 1. Function values are not resolved.
func nilFactory() *Config {
	return nil
}

func functionValueNil() {
	factory := nilFactory
	cfg := factory()
	_ = cfg.Host // Potential nil pointer dereference.
}

// 2. Methods are not resolved.
type Provider struct{}

func (p *Provider) GetConfig() *Config {
	return nil
}

func methodNil() {
	var provider Provider
	cfg := provider.GetConfig()
	_ = cfg.Host // Potential nil pointer dereference.
}

// 3. Imported functions are not resolved.
func importedFunctionNil() {
	// Example to be added when testing an imported package:
	// cfg := external.GetConfig()
	// _ = cfg.Host
}

// ============================================================
// NILPTR LIMITATIONS
// ============================================================

// 4. Pointer values through interfaces.
func interfaceNil() {
	var cfg *Config
	var value any = cfg

	actual := value.(*Config)
	_ = actual.Host // Nil pointer dereference.
}

// 5. Pointer values through maps.
func mapNil() {
	configs := map[string]*Config{
		"server": nil,
	}

	cfg := configs["server"]
	_ = cfg.Host // Nil pointer dereference.
}

// 6. Pointer values through channels.
func channelNil() {
	ch := make(chan *Config, 1)
	ch <- nil

	cfg := <-ch
	_ = cfg.Host // Nil pointer dereference.
}

// 7. Pointer values through goroutines.
func goroutineNil() {
	var cfg *Config

	go func() {
		_ = cfg.Host // Nil pointer dereference.
	}()
}

// ============================================================
// DIVZERO LIMITATIONS
// ============================================================

// 8. Package-level variables.
var divisor = 0

func packageVariableDivision() int {
	return 100 / divisor
}

// 9. Values reached through pointers.
func pointerDivision() int {
	zero := 0
	divisor := &zero

	return 100 / *divisor
}

// 10. Function results.
func getDivisor() int {
	return 0
}

func functionResultDivision() int {
	return 100 / getDivisor()
}

// 11. Type parameters.
func genericDivision[T ~int](a, b T) T {
	return a / b
}

func genericDivisionCall() int {
	return genericDivision(100, 0)
}

// ============================================================
// BOUNDS LIMITATIONS
// ============================================================

// 12. Capacity tracking.
func capacityBounds() int {
	values := make([]int, 1, 4)
	values = values[:cap(values)]

	return values[3]
}

// 13. Loop induction values.
func loopBounds() int {
	values := []int{10, 20, 30}

	for i := 0; i <= len(values); i++ {
		if i == len(values) {
			return values[i] // Out of bounds.
		}
	}

	return 0
}

// 14. Interprocedural length tracking.
func readIndex(values []int, index int) int {
	return values[index]
}

func interproceduralBounds() int {
	values := []int{10, 20, 30}
	index := 5

	return readIndex(values, index) // Out of bounds.
}

// ============================================================
// ASSERT LIMITATIONS
// ============================================================

type Envelope struct {
	Value any
}

// 15. Dynamic types through struct fields.
func assertionThroughField() string {
	envelope := Envelope{Value: 123}

	return envelope.Value.(string)
}

// 16. Dynamic types through maps.
func assertionThroughMap() string {
	values := map[string]any{
		"name": 123,
	}

	return values["name"].(string)
}

// 17. Dynamic types through channels.
func assertionThroughChannel() string {
	ch := make(chan any, 1)
	ch <- 123

	value := <-ch
	return value.(string)
}

// 18. Dynamic types through type parameters.
func assertionThroughGeneric[T any](value T) string {
	return any(value).(string)
}

func genericAssertionCall() string {
	return assertionThroughGeneric(123)
}

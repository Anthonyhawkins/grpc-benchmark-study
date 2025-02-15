package calculation

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// Calculation holds the information for a calculation request and response.
type Calculation struct {
	ID        int32  `json:"id"`
	X         int    `json:"x"`
	Y         int    `json:"y"`
	Operation string `json:"operation"`
	Result    int    `json:"result"`
	Prime     bool   `json:"isPrime"`
}

func Read(input []byte) (*Calculation, error) {
	var calc Calculation
	if err := json.Unmarshal(input, &calc); err != nil {
		return nil, err
	}
	return &calc, nil
}

// PerformCalculation unmarshals the input JSON into a Calculation,
// determines which operation to perform, executes it, and returns the
// resulting Calculation as JSON.
func PerformCalculation(input []byte) ([]byte, error) {
	var calc Calculation
	if err := json.Unmarshal(input, &calc); err != nil {
		return nil, err
	}

	// Normalize the operation string to uppercase.
	op := strings.ToUpper(calc.Operation)
	switch op {
	case "ADD":
		calc.Add()
	case "SUBTRACT":
		calc.Sub()
	case "ISPRIME":
		calc.IsPrime()
	default:
		return nil, fmt.Errorf("unknown operation: %s", calc.Operation)
	}

	return calc.Bytes()
}

// Add performs an addition of X and Y, storing the result.
func (c *Calculation) Add() {
	c.Result = c.X + c.Y
}

// Sub performs a subtraction (X - Y), storing the result.
func (c *Calculation) Sub() {
	c.Result = c.X - c.Y
}

// IsPrime determines if X is a prime number and stores the result in Prime.
func (c *Calculation) IsPrime() {
	if c.X <= 1 {
		c.Prime = false
		return
	}
	// Check divisibility from 2 up to sqrt(X).
	sqrtX := int(math.Sqrt(float64(c.X)))
	for i := 2; i <= sqrtX; i++ {
		if c.X%i == 0 {
			c.Prime = false
			return
		}
	}
	c.Prime = true
}

// Bytes serializes the Calculation to JSON.
func (c *Calculation) Bytes() ([]byte, error) {
	return json.Marshal(c)
}

// String returns a key=value representation of the Calculation.
func (c *Calculation) String() string {
	return fmt.Sprintf("id=%d, x=%d, y=%d, operation=%s, result=%d, isPrime=%t",
		c.ID, c.X, c.Y, c.Operation, c.Result, c.Prime)
}

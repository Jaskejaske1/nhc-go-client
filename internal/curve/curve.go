package curve

import (
	"fmt"
	"math"
)

const (
	MinPercent = 0
	MaxPercent = 100
)

type Mapper interface {
	Map(percent int) (int, error)
	Name() string
}

type Linear struct{}

func (Linear) Map(percent int) (int, error) {
	if err := validatePercent(percent); err != nil {
		return 0, err
	}
	return percent, nil
}

func (Linear) Name() string {
	return "linear"
}

type Gamma struct {
	Exponent float64
}

func NewGamma(exponent float64) (Gamma, error) {
	if exponent <= 0 || math.IsNaN(exponent) || math.IsInf(exponent, 0) {
		return Gamma{}, fmt.Errorf("gamma exponent must be finite and greater than zero")
	}
	return Gamma{Exponent: exponent}, nil
}

func (g Gamma) Map(percent int) (int, error) {
	if err := validatePercent(percent); err != nil {
		return 0, err
	}
	if g.Exponent <= 0 || math.IsNaN(g.Exponent) || math.IsInf(g.Exponent, 0) {
		return 0, fmt.Errorf("gamma exponent must be finite and greater than zero")
	}
	if percent == MinPercent || percent == MaxPercent {
		return percent, nil
	}
	return int(math.Round(math.Pow(float64(percent)/MaxPercent, g.Exponent) * MaxPercent)), nil
}

func (g Gamma) Name() string {
	return fmt.Sprintf("gamma(%g)", g.Exponent)
}

type Point struct {
	Input  int `json:"input"`
	Output int `json:"output"`
}

type Lookup struct {
	Points []Point
}

func NewLookup(points []Point) (Lookup, error) {
	if len(points) < 2 {
		return Lookup{}, fmt.Errorf("lookup curve requires at least two points")
	}
	previousInput := -1
	previousOutput := -1
	for _, point := range points {
		if err := validatePercent(point.Input); err != nil {
			return Lookup{}, fmt.Errorf("invalid lookup input: %w", err)
		}
		if err := validatePercent(point.Output); err != nil {
			return Lookup{}, fmt.Errorf("invalid lookup output: %w", err)
		}
		if point.Input <= previousInput {
			return Lookup{}, fmt.Errorf("lookup inputs must be strictly increasing")
		}
		if point.Output < previousOutput {
			return Lookup{}, fmt.Errorf("lookup outputs must be monotonic")
		}
		previousInput = point.Input
		previousOutput = point.Output
	}
	if points[0].Input != MinPercent || points[len(points)-1].Input != MaxPercent {
		return Lookup{}, fmt.Errorf("lookup curve must start at 0 and end at 100")
	}
	if points[0].Output != MinPercent || points[len(points)-1].Output != MaxPercent {
		return Lookup{}, fmt.Errorf("lookup curve outputs must start at 0 and end at 100")
	}
	return Lookup{Points: append([]Point(nil), points...)}, nil
}

func (l Lookup) Map(percent int) (int, error) {
	if err := validatePercent(percent); err != nil {
		return 0, err
	}
	if len(l.Points) < 2 {
		return 0, fmt.Errorf("lookup curve is not configured")
	}
	for i := 1; i < len(l.Points); i++ {
		left := l.Points[i-1]
		right := l.Points[i]
		if percent <= right.Input {
			inputRange := right.Input - left.Input
			outputRange := right.Output - left.Output
			return left.Output + (percent-left.Input)*outputRange/inputRange, nil
		}
	}
	return l.Points[len(l.Points)-1].Output, nil
}

func (l Lookup) Name() string {
	return "lookup"
}

func validatePercent(percent int) error {
	if percent < MinPercent || percent > MaxPercent {
		return fmt.Errorf("brightness must be between %d and %d, got %d", MinPercent, MaxPercent, percent)
	}
	return nil
}

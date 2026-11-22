package model

import (
	"fmt"
	"math"
	"sort"
)

// Computed makes a number variable a computed value: its value is calculated from its inputs,
// by one function of a closed list, whenever an input reports. Nobody can write it.
type Computed struct {
	Function string  `json:"function"`
	Inputs   []Input `json:"inputs"`
}

// Input is one input of a computed value: a device datapoint or a variable (a number, a system
// value or another computed value).
type Input struct {
	Device   *Ref   `json:"device,omitempty"`
	Variable string `json:"variable,omitempty"`
}

// The functions of a computed value.
const (
	FnDewPoint         = "dew-point"         // temperature (°C), relative humidity (%) -> °C
	FnAbsoluteHumidity = "absolute-humidity" // temperature (°C), relative humidity (%) -> g/m³
	FnDifference       = "difference"        // first minus second
	FnSum              = "sum"
	FnAverage          = "average"
	FnMin              = "min"
	FnMax              = "max"
)

// ComputedFunctions lists the functions, in the order a form offers them.
var ComputedFunctions = []string{FnDewPoint, FnAbsoluteHumidity, FnDifference, FnSum, FnAverage, FnMin, FnMax}

// MaxInputs is the most inputs a sum, an average, a minimum or a maximum takes.
const MaxInputs = 8

// InputCount answers how many inputs a function takes, at least and at most; 0, 0 for a
// function that does not exist.
func InputCount(function string) (min, max int) {
	switch function {
	case FnDewPoint, FnAbsoluteHumidity, FnDifference:
		return 2, 2
	case FnSum, FnAverage, FnMin, FnMax:
		return 2, MaxInputs
	}
	return 0, 0
}

// Source names an input the way the engine names what reports: "dp:<ref>" or "var:<key>".
func (in Input) Source() string {
	if in.Device != nil {
		return "dp:" + in.Device.String()
	}
	return "var:" + in.Variable
}

// The Magnus formula's constants over water (Sonntag 1990), good from -45 to 60 °C.
const (
	magnusA = 17.62
	magnusB = 243.12
)

func round(x float64, digits int) float64 {
	p := math.Pow(10, float64(digits))
	return math.Round(x*p) / p
}

// Compute calculates a function of its inputs' values. ok is false when it cannot be said: a
// function that does not exist, the wrong number of inputs, a humidity of nothing or less, a
// result that is no number. Dew point and absolute humidity are rounded to a tenth, the others
// to six decimals (so 21.3 - 19.8 is 1.5).
func Compute(function string, in []float64) (value float64, ok bool) {
	lo, hi := InputCount(function)
	if lo == 0 || len(in) < lo || len(in) > hi {
		return 0, false
	}
	switch function {
	case FnDewPoint, FnAbsoluteHumidity:
		t, rh := in[0], in[1]
		if rh <= 0 || t <= -magnusB {
			return 0, false
		}
		if function == FnDewPoint {
			g := math.Log(rh/100) + magnusA*t/(magnusB+t)
			value = round(magnusB*g/(magnusA-g), 1)
		} else {
			// the vapour pressure in hPa, then the mass of water in a cubic metre of air
			e := rh / 100 * 6.112 * math.Exp(magnusA*t/(magnusB+t))
			value = round(216.7*e/(273.15+t), 1)
		}
	case FnDifference:
		value = round(in[0]-in[1], 6)
	default:
		sorted := append([]float64(nil), in...)
		sort.Float64s(sorted)
		sum := 0.0
		for _, x := range in {
			sum += x
		}
		switch function {
		case FnSum:
			value = round(sum, 6)
		case FnAverage:
			value = round(sum/float64(len(in)), 6)
		case FnMin:
			value = sorted[0]
		default:
			value = sorted[len(sorted)-1]
		}
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return value, true
}

// ComputedProblem says what keeps a computed value from being calculated, as far as the set
// can tell: an input variable that does not exist or is no number, or inputs that lead back
// to the variable itself. Empty when nothing does, and for a variable that is not computed.
func (s *Set) ComputedProblem(v *Variable) string {
	if v.Computed == nil {
		return ""
	}
	for _, in := range v.Computed.Inputs {
		if in.Device != nil {
			continue
		}
		if in.Variable == v.Key {
			return fmt.Sprintf("%s cannot be its own input", v.Key)
		}
		if IsSystemValue(in.Variable) {
			continue
		}
		iv := s.Variables[in.Variable]
		if iv == nil {
			return fmt.Sprintf("the input variable %s does not exist", in.Variable)
		}
		if iv.Type != VarNumber {
			return fmt.Sprintf("the input variable %s is not a number", in.Variable)
		}
	}
	// a circle: follow the inputs' inputs
	seen := map[string]bool{}
	var leads func(key string) bool
	leads = func(key string) bool {
		if key == v.Key {
			return true
		}
		if seen[key] {
			return false
		}
		seen[key] = true
		iv := s.Variables[key]
		if iv == nil || iv.Computed == nil {
			return false
		}
		for _, in := range iv.Computed.Inputs {
			if in.Device == nil && leads(in.Variable) {
				return true
			}
		}
		return false
	}
	for _, in := range v.Computed.Inputs {
		if in.Device == nil && leads(in.Variable) {
			return fmt.Sprintf("the inputs of %s lead back to itself through %s", v.Key, in.Variable)
		}
	}
	return ""
}

// ComputedUsers lists the computed values that have the variable as an input, by name, sorted.
func (s *Set) ComputedUsers(key string) []string {
	out := []string{}
	for _, v := range s.Variables {
		if v.Computed == nil {
			continue
		}
		for _, in := range v.Computed.Inputs {
			if in.Device == nil && in.Variable == key {
				out = append(out, v.Name)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

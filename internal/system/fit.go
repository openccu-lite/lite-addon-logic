package system

import (
	"fmt"
	"math"
	"strconv"

	"github.com/hobbyquaker/ccu-addon-kit/go/litesource"
)

// paramOf is a datapoint as its description says it.
func paramOf(id string, d litesource.ParamDesc) Param {
	return Param{ID: id, Type: d.Type, Read: d.Operations&5 != 0, Write: d.Operations&2 != 0,
		Min: number(d.Min), Max: number(d.Max), Unit: d.Unit, Values: d.ValueList}
}

func short(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// Fit says whether a datapoint takes a value, and answers the value in the form the
// description asks for: the index of an ENUM's entry for its name, a number for true and false
// where a number is asked, a whole number for an INTEGER. What it refuses: a value of the wrong
// kind, a number outside the datapoint's limits, an entry the list does not have. Whether the
// datapoint can be written at all is the caller's question (Param.Write).
func (p Param) Fit(value any) (any, error) {
	num, isNum := value.(float64)
	b, isBool := value.(bool)
	text, isText := value.(string)
	if isBool && (p.Type == "FLOAT" || p.Type == "INTEGER" || p.Type == "ENUM") {
		num, isNum = map[bool]float64{true: 1, false: 0}[b], true
	}
	within := func() error {
		if (p.Min != nil && num < *p.Min) || (p.Max != nil && num > *p.Max) {
			lo, hi := "…", "…"
			if p.Min != nil {
				lo = short(*p.Min)
			}
			if p.Max != nil {
				hi = short(*p.Max)
			}
			return fmt.Errorf("%s: %s is outside %s to %s", p.ID, short(num), lo, hi)
		}
		return nil
	}
	switch p.Type {
	case "BOOL", "ACTION":
		switch {
		case isBool:
			return b, nil
		case isNum && (num == 0 || num == 1):
			return num == 1, nil
		}
		return nil, fmt.Errorf("%s takes true or false", p.ID)
	case "FLOAT":
		if !isNum {
			return nil, fmt.Errorf("%s takes a number", p.ID)
		}
		return litesource.Double(num), within()
	case "INTEGER":
		if !isNum {
			return nil, fmt.Errorf("%s takes a whole number", p.ID)
		}
		num = math.Round(num)
		return int(num), within()
	case "ENUM":
		if isText {
			for i, entry := range p.Values {
				if entry == text && entry != "" {
					return i, nil
				}
			}
			return nil, fmt.Errorf("%s has no entry %q", p.ID, text)
		}
		if !isNum {
			return nil, fmt.Errorf("%s takes one of its entries", p.ID)
		}
		num = math.Round(num)
		if num < 0 || (len(p.Values) > 0 && int(num) >= len(p.Values)) {
			return nil, fmt.Errorf("%s has no entry number %s", p.ID, short(num))
		}
		return int(num), nil
	case "STRING":
		if isText {
			return text, nil
		}
		return fmt.Sprint(value), nil
	}
	// a type this addon does not know: the value as the file has it
	if isNum && num == math.Trunc(num) {
		return int(num), nil
	}
	if isNum {
		return litesource.Double(num), nil
	}
	return value, nil
}

// FitAll checks the values an activity writes to one channel against the channel's datapoints
// (as Catalog.Params lists them): each must be there, writable, and fit. The error names the
// first that is not.
func FitAll(params []Param, values map[string]any) error {
	byID := map[string]Param{}
	for _, p := range params {
		byID[p.ID] = p
	}
	for id, v := range values {
		p, ok := byID[id]
		if !ok {
			return fmt.Errorf("the channel has no datapoint %s", id)
		}
		if !p.Write {
			return fmt.Errorf("%s cannot be written", id)
		}
		if _, err := p.Fit(v); err != nil {
			return err
		}
	}
	return nil
}

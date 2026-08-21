package converter

import (
	"fmt"
	"strconv"
)

// Convert converts a value of any supported type to T.
func Convert[T any](value any) (T, error) {
	var zero T
	result, err := convertTo(value, zero)
	if err != nil {
		return zero, err
	}
	converted, ok := result.(T)
	if !ok {
		return zero, fmt.Errorf("cannot convert %T to %T", value, zero)
	}
	return converted, nil
}

func convertTo(value any, target any) (any, error) {
	switch target.(type) {
	case string:
		return toString(value)
	case int:
		return toInt(value)
	case int64:
		return toInt64(value)
	case float64:
		return toFloat64(value)
	case bool:
		return toBool(value)
	default:
		return nil, fmt.Errorf("unsupported target type %T", target)
	}
}

func toString(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	case int:
		return strconv.Itoa(v), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case bool:
		return strconv.FormatBool(v), nil
	case fmt.Stringer:
		return v.String(), nil
	default:
		return fmt.Sprint(v), nil
	}
}

func toInt(value any) (int, error) {
	n, err := toInt64(value)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func toInt64(value any) (int64, error) {
	switch v := value.(type) {
	case int:
		return int64(v), nil
	case int8:
		return int64(v), nil
	case int16:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int64:
		return v, nil
	case uint:
		return int64(v), nil
	case uint8:
		return int64(v), nil
	case uint16:
		return int64(v), nil
	case uint32:
		return int64(v), nil
	case uint64:
		return int64(v), nil
	case float32:
		return int64(v), nil
	case float64:
		return int64(v), nil
	case bool:
		if v {
			return 1, nil
		}
		return 0, nil
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("cannot convert %q to int64: %w", v, err)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("cannot convert %T to int64", value)
	}
}

func toFloat64(value any) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	case bool:
		if v {
			return 1, nil
		}
		return 0, nil
	case string:
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, fmt.Errorf("cannot convert %q to float64: %w", v, err)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", value)
	}
}

func toBool(value any) (bool, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case int, int8, int16, int32, int64:
		n, _ := toInt64(v)
		return n != 0, nil
	case uint, uint8, uint16, uint32, uint64:
		n, _ := toInt64(v)
		return n != 0, nil
	case float32, float64:
		f, _ := toFloat64(v)
		return f != 0, nil
	case string:
		b, err := strconv.ParseBool(v)
		if err != nil {
			return false, fmt.Errorf("cannot convert %q to bool: %w", v, err)
		}
		return b, nil
	default:
		return false, fmt.Errorf("cannot convert %T to bool", value)
	}
}

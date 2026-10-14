package model

import "strconv"

// itoa formats an integer without fmt overhead in hot paths.
func itoa(n int) string { return strconv.Itoa(n) }

func trimFloat(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

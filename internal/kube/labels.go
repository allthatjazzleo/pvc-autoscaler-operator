package kube

import "bytes"

const (
	OperatorEnabled   = "pvc-autoscaler-operator.kubernetes.io/enabled"
	OperatorName      = "pvc-autoscaler-operator.kubernetes.io/operator-name"
	OperatorNamespace = "pvc-autoscaler-operator.kubernetes.io/operator-namespace"
	OperatorImage     = "pvc-autoscaler-operator.kubernetes.io/sidecar-image"
)

// Fields.
const (
	ControllerField = ".spec.podDiskInspector"
)

// ToName normalizes val per kubernetes name constraints to a max of 253 characters.
// See: https://unofficial-kubernetes.readthedocs.io/en/latest/concepts/overview/working-with-objects/names/
func ToName(val string) string {
	return normalizeValue(val, 253, '-', '.')
}

func normalizeValue(val string, limit int, allowed ...byte) string {
	// Select only alphanumeric and allowed characters.
	result := []byte(val)
	j := 0
	for _, char := range []byte(val) {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			(bytes.IndexByte(allowed, char) != -1) {
			result[j] = char
			j++
		}
	}
	result = result[:j]

	// Start and end with alphanumeric only
	result = bytes.TrimLeftFunc(result, func(r rune) bool {
		return bytes.ContainsRune(allowed, r)
	})
	result = bytes.TrimRightFunc(result, func(r rune) bool {
		return bytes.ContainsRune(allowed, r)
	})

	return trimMiddle(string(result), limit)
}

// Truncates the middle, trying to preserve prefix and suffix.
func trimMiddle(val string, limit int) string {
	if len(val) <= limit {
		return val
	}

	// Truncate the middle, trying to preserve prefix and suffix.
	left, right := limit/2, limit/2
	if limit%2 != 0 {
		right++
	}
	b := []byte(val)
	return string(append(b[:left], b[len(b)-right:]...))
}

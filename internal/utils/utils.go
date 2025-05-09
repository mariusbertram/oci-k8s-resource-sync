package utils

// FilterMapInPlace filters a map in-place by keeping only the keys that are present in the allowedKeys slice.
// This function modifies the original map directly without creating a new one.
//
// Parameters:
//   - m: The map to be filtered. It contains string keys and byte slice values.
//   - allowedKeys: A slice of strings representing the keys that should be kept in the map.
//
// How it works:
// 1. Creates a set (implemented as a map with empty struct values) from the allowedKeys for efficient lookups
// 2. Iterates through all keys in the original map
// 3. Deletes any key that is not present in the allowedSet
//
// This is useful for restricting a map to only contain specific keys, such as when
// filtering files or configuration data to include only what's needed.
func FilterMapInPlace(m map[string][]byte, allowedKeys []string) {

	// Create a set from allowedKeys for O(1) lookups
	allowedSet := make(map[string]struct{})
	for _, key := range allowedKeys {
		allowedSet[key] = struct{}{}
	}

	// Remove any key from the map that is not in the allowed set
	for key := range m {
		if _, ok := allowedSet[key]; !ok {
			delete(m, key)
		}
	}
}

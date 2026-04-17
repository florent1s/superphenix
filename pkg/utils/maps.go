package utils

// MapDeepMerge merges src into dst recursively, with src taking precedence.
// A nil value in src deletes the corresponding key from dst, which allows
// callers to explicitly drop Helm chart values via YAML null.
func MapDeepMerge(dst, src map[string]interface{}) {
	for key, value := range src {
		if value == nil {
			delete(dst, key)
			continue
		}
		if srcMap, ok := value.(map[string]interface{}); ok {
			if dstMap, ok := dst[key].(map[string]interface{}); ok {
				MapDeepMerge(dstMap, srcMap)
				continue
			}
		}
		dst[key] = value
	}
}

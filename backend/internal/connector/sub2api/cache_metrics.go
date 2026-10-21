package sub2api

func cacheTokenValue(value *int64) int64 {
	if value == nil || *value < 0 {
		return 0
	}
	return *value
}

package server

func trimPattern(glob string) string {
	escaped := false
	pattern := ""
parse:
	for _, char := range glob {
		switch char {
		case '\\':
			escaped = !escaped
			if !escaped {
				pattern = pattern + string(char)
			}
		case '*':
			if !escaped {
				break parse
			}
			pattern = pattern + string(char)
		case '?':
			if !escaped {
				break parse
			}
			pattern = pattern + string(char)
		case '[':
			if !escaped {
				break parse
			}
			pattern = pattern + string(char)
		default:
			pattern = pattern + string(char)
		}
	}
	return pattern
}

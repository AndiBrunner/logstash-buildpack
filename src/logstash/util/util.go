package util

import "regexp"

func TrimLines(text string) string {
	re := regexp.MustCompile("(?m)^(\\s)*")
	return re.ReplaceAllString(text, "")
}

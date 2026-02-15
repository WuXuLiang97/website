package utils

import (
	"path"
	"strings"
)

func NormalizeURLPath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	parts := strings.FieldsFunc(p, func(r rune) bool {
		return r == '/'
	})
	if len(parts) == 0 {
		return "/"
	}
	return "/" + strings.Join(parts, "/")
}

func IsVideoFile(filePath string, allowedFormats []string) bool {
	ext := strings.ToLower(path.Ext(filePath))
	for _, format := range allowedFormats {
		if ext == format {
			return true
		}
	}
	return false
}

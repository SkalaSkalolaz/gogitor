package textutil

import (
	"unicode/utf8"
)

// LimitRunes обрезает строку до maxRunes рун, не разрывая UTF-8 символы.
// suffix добавляется только если строка была обрезана.
//
// Важно: лимит здесь считается именно в рунах, а не в байтах.
func LimitRunes(s string, maxRunes int, suffix string) string {
	if maxRunes < 0 {
		maxRunes = 0
	}

	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}

	return string(runes[:maxRunes]) + suffix
}

// TruncateStringBytes обрезает строку так, чтобы она занимала не больше
// maxBytes байт, но не разрезает многобайтовый UTF-8 символ.
//
// Этот вариант подходит там, где лимит действительно байтовый,
// например контекст для LLM или diff.
func TruncateStringBytes(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}

	if len(s) <= maxBytes {
		return s
	}

	i := maxBytes

	// Откатываемся назад, пока не окажемся на границе руны.
	for i > 0 && !utf8.RuneStart(s[i]) {
		i--
	}

	return s[:i]
}

// TruncateBytes делает то же самое для []byte.
func TruncateBytes(b []byte, maxBytes int) []byte {
	if maxBytes <= 0 {
		return nil
	}

	if len(b) <= maxBytes {
		return b
	}

	i := maxBytes

	// Откатываемся назад, пока не окажемся на границе руны.
	for i > 0 && !utf8.RuneStart(b[i]) {
		i--
	}

	return b[:i]
}
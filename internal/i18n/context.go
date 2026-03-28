package i18n

import (
	"context"
	"strings"
	"time"
)

type contextKey string

const (
	localeKey   contextKey = "request-locale"
	timezoneKey contextKey = "request-timezone"
)

func NormalizeLocale(raw string) string {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return "en"
	}
	trimmed = strings.ReplaceAll(trimmed, "_", "-")
	trimmed = strings.Split(trimmed, ",")[0]
	trimmed = strings.Split(trimmed, ";")[0]
	parts := strings.Split(trimmed, "-")
	base := strings.TrimSpace(parts[0])
	if base == "" {
		return "en"
	}
	return base
}

func ParseLocale(headerLocale string, acceptLanguage string) string {
	if strings.TrimSpace(headerLocale) != "" {
		return NormalizeLocale(headerLocale)
	}
	for _, candidate := range strings.Split(acceptLanguage, ",") {
		if locale := NormalizeLocale(candidate); locale != "" {
			return locale
		}
	}
	return "en"
}

func NormalizeTimezone(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if _, err := time.LoadLocation(value); err != nil {
		return ""
	}
	return value
}

func WithRequestContext(ctx context.Context, locale, timezone string) context.Context {
	next := context.WithValue(ctx, localeKey, NormalizeLocale(locale))
	return context.WithValue(next, timezoneKey, NormalizeTimezone(timezone))
}

func LocaleFromContext(ctx context.Context) string {
	value, ok := ctx.Value(localeKey).(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "en"
	}
	return NormalizeLocale(value)
}

func TimezoneFromContext(ctx context.Context) string {
	value, ok := ctx.Value(timezoneKey).(string)
	if !ok {
		return ""
	}
	return NormalizeTimezone(value)
}

func MessageLanguage(locale string) string {
	if NormalizeLocale(locale) == "zh" {
		return "zh"
	}
	return "en"
}

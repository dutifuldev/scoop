package translation

import (
	"sort"
	"strings"
)

type LanguageOption struct {
	Code   string `json:"code"`
	Label  string `json:"label"`
	Native string `json:"native,omitempty"`
}

type languageLabel struct {
	english string
	chinese string
}

var translationLanguageLabels = map[string]languageLabel{
	"ar": {english: "Arabic", chinese: "阿拉伯语"},
	"de": {english: "German", chinese: "德语"},
	"en": {english: "English", chinese: "英语"},
	"es": {english: "Spanish", chinese: "西班牙语"},
	"fr": {english: "French", chinese: "法语"},
	"id": {english: "Indonesian", chinese: "印度尼西亚语"},
	"it": {english: "Italian", chinese: "意大利语"},
	"ja": {english: "Japanese", chinese: "日语"},
	"ko": {english: "Korean", chinese: "韩语"},
	"pl": {english: "Polish", chinese: "波兰语"},
	"pt": {english: "Portuguese", chinese: "葡萄牙语"},
	"ru": {english: "Russian", chinese: "俄语"},
	"th": {english: "Thai", chinese: "泰语"},
	"tr": {english: "Turkish", chinese: "土耳其语"},
	"vi": {english: "Vietnamese", chinese: "越南语"},
	"zh": {english: "Chinese", chinese: "中文"},
}

func SupportedTranslationLanguageCodes() []string {
	codes := make([]string, 0, len(translationLanguageLabels))
	for code := range translationLanguageLabels {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}

func ViewerLanguageOptions(registry *Registry) []LanguageOption {
	options := []LanguageOption{
		{
			Code:  "original",
			Label: "Original",
		},
	}
	options = append(options, TranslationLanguageOptions(registry)...)
	return options
}

func TranslationLanguageOptions(registry *Registry) []LanguageOption {
	codes := supportedTranslationCodes(registry)
	options := make([]LanguageOption, 0, len(codes))
	for _, code := range codes {
		options = append(options, languageOptionForCode(code))
	}
	return options
}

func supportedTranslationCodes(registry *Registry) []string {
	supported := map[string]struct{}{}

	for code := range translationLanguageLabels {
		addSupportedLanguageCode(supported, code)
	}

	if registry != nil {
		for _, provider := range registry.providers {
			for _, code := range provider.SupportedLanguages() {
				addSupportedLanguageCode(supported, code)
			}
		}
	}

	codes := make([]string, 0, len(supported))
	for code := range supported {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}

func addSupportedLanguageCode(supported map[string]struct{}, code string) {
	normalized := normalizeLangCode(code)
	if normalized != "" {
		supported[normalized] = struct{}{}
	}
}

func languageOptionForCode(code string) LanguageOption {
	labels, hasLabels := translationLanguageLabels[code]
	if hasLabels {
		return LanguageOption{
			Code:   code,
			Label:  labels.english,
			Native: labels.chinese,
		}
	}
	return LanguageOption{
		Code:  code,
		Label: strings.ToUpper(code),
	}
}

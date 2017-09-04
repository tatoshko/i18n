package i18n

import (
	"strings"
)

const (
	COMMON_DATA = "root"
)

type Source interface {
	GetRules(locale string) ([]byte, bool)
	GetMessages(key, locale string) (string, bool)
}

// TranslatorFactory is a struct which contains the info necessary for creating
// Translator "instances". It also "caches" previously created Translators.
// Because of this caching, you can request a Translator for a specific locale
// multiple times and always get a pointer to the same Translator instance.
type TranslatorFactory struct {
	source        Source
	translators   map[string]*Translator
	fallback      *Translator
}

// Translator is a struct which contains all the rules and messages necessary
// to do internationalization for a specific locale. Most functionality in this
// package is accessed through a Translator instance.
type Translator struct {
	messages func(key, locale string) (string, bool)
	locale   string
	rules    *TranslatorRules
	fallback *Translator
}

// translatorError implements the error interface for use in this package. it
// keeps an optional reference to a Translator instance, which it uses to
// include which locale the error occurs with in the error message returned by
// the Error() method
type translatorError struct {
	translator *Translator
	message    string
}

// Error satisfies the error interface requirements
func (e translatorError) Error() string {
	if e.translator != nil {
		return "translator error (locale: " + e.translator.locale + ") - " + e.message
	}
	return "translator error - " + e.message
}

// NewTranslatorFactory returns a TranslatorFactory instance with the specified
// paths and fallback locale.  If a fallback locale is specified, it
// automatically creates the fallback Translator instance. Several errors can
// occur during this process, and those are all returned in the errors slice.
// Even if errors are returned, this function should still return a working
// Translator if the fallback works.
//
// If multiple rulesPaths or messagesPaths are provided, they loaded in the
// order they appear in the slice, with values added later overriding any rules
// or messages loaded earlier.
//
// One lat thing about the messagesPaths. You can organize your locale messages
// files in this messagesPaths directory in 2 different ways.
//
//  1) Place *.yaml files in that directory directly, named after locale codes -
//
//     messages/
//       en.yaml
//       fr.yaml
//
//  2) Place subdirectores in that directory, named after locale codes and
//     containing *.yaml files
//
//     messages/
//       en/
//         front-end.yaml
//         email.yaml
//       fr/
//         front-end.yaml
//         email.yaml
//
//  Using the second way allows you to organize your messages into multiple
//  files.
func NewTranslatorFactory(source Source, fallbackLocale string) (f *TranslatorFactory, errors []error) {
	f = new(TranslatorFactory)

	f.source = source
	f.translators = map[string]*Translator{}

	// load and check the fallback locale
	if fallbackLocale != "" {
		var errs []error
		f.fallback, errs = f.GetTranslator(fallbackLocale)
		for _, err := range errs {
			errors = append(errors, err)
		}
	}

	return
}

// GetTranslator returns an Translator instance for the requested locale. If you
// request the same locale multiple times, a pointed to the same Translator will
// be returned each time.
func (f *TranslatorFactory) GetTranslator(localeCode string) (t *Translator, errors []error) {

	fallback := f.getFallback(localeCode)

	if t, ok := f.translators[localeCode]; ok {
		return t, nil
	}

	rules := new(TranslatorRules)

	data, ok := f.source.GetRules(COMMON_DATA)

	if !ok {
		errors = append(errors, translatorError{message: "could not find root (COMMON_DATA)"})
	}

	if errs := rules.load(data, false, false); errs != nil {
		errors = append(errors, errs...)
	}

	data, ok = f.source.GetRules(localeCode)

	if !ok {
		errors = append(errors, translatorError{message: "could not find rules and messages for locale " + localeCode})
	}

	if errs := rules.load(data, true, true); errs != nil {
		errors = append(errors, errs...)
	}

	t = new(Translator)
	t.locale = localeCode
	t.fallback = fallback
	t.rules = rules
	t.messages = f.source.GetMessages

	f.translators[localeCode] = t

	return
}

// getFallback returns the best fallback for this locale. It first checks for
// less specific versions of the locale before falling back to the global
// fallback if it exists.
func (f *TranslatorFactory) getFallback(localeCode string) *Translator {

	if f.fallback != nil && localeCode == f.fallback.locale {
		return nil
	}

	separator := "-"

	// for a "multipart" locale code, find the most appropriate fallback
	// start by taking off the last "part"
	// if you run out of parts, use the factory's fallback

	fallback := f.fallback
	parts := strings.Split(localeCode, separator)
	for len(parts) > 1 {
		parts = parts[0 : len(parts)-1]
		fb := strings.Join(parts, separator)

		if exists := f.LocaleExists(fb); exists {
			fallback, _ = f.GetTranslator(fb)
			break
		}
	}

	return fallback
}

// LocaleExists checks to see if any messages files exist for the requested
// locale string.
func (f *TranslatorFactory) LocaleExists(localeCode string) (exists bool) {
	_, exists = f.source.GetRules(localeCode)

	return
}

func (t *Translator) Tr(key string, substitutions map[string]string) string {
	translation, errors := t.Translate(key, substitutions)

	if errors != nil {
		return key
	}

	return translation
}

func (t *Translator) T(key string) string {
	return t.Tr(key, map[string]string{})
}

func (t *Translator) Err(key string) error {
	return fmt.Errorf("%s", t.T(key))
}

// Translate returns the translated message, performang any substitutions
// requested in the substitutions map. If neither this translator nor its
// fallback translator (or the fallback's fallback and so on) have a translation
// for the requested key, and empty string and an error will be returned.
func (t *Translator) Translate(key string, substitutions map[string]string) (translation string, errors []error) {
	message, exists := t.messages(key, t.locale)

	if exists {
		translation, errors = t.substitute(message, substitutions)
		return
	}

	if t.fallback != nil && t.fallback != t {
		return t.fallback.Translate(key, substitutions)
	}

	errors = append(errors, translatorError{translator: t, message: "key not found: " + key})

	return
}

func (t *Translator) P(key string, number float64, numberStr string) string {
	translation, errors := t.Pluralize(key, number, numberStr)

	if errors != nil {
		translation = key
	}

	return translation
}

// Pluralize returns the translation for a message containing a plural. The
// plural form used is based on the number float64 and the number displayed in
// the translated string is the numberStr string. If neither this translator nor
// its fallback translator (or the fallback's fallback and so on) have a
// translation for the requested key, and empty string and an error will be
// returned.
func (t *Translator) Pluralize(key string, number float64, numberStr string) (translation string, errors []error) {
	message, exists := t.messages(key, t.locale)

	if ! exists {
		if t.fallback != nil && t.fallback != t {
			return t.fallback.Pluralize(key, number, numberStr)
		}

		errors = append(errors, translatorError{translator: t, message: "key not found: " + key})
		return
	}

	form := (t.rules.PluralRuleFunc)(number)

	parts := strings.Split(message, "|")

	if form > len(parts)-1 {
		errors = append(errors, translatorError{translator: t, message: "too few plural variations: " + key})
		form = len(parts) - 1
	}

	var errs []error
	translation, errs = t.substitute(parts[form], map[string]string{"n": numberStr})
	for _, err := range errs {
		errors = append(errors, err)
	}
	return
}

// Translate returns the translated message, performang any substitutions
// requested in the substitutions map. If neither this translator nor its
// fallback translator (or the fallback's fallback and so on) have a translation
// for the requested key, and empty string and an error will be returned.
func (t *Translator) Rules() TranslatorRules {
	rules := *t.rules

	return rules
}

// Direction returns the text directionality of the locale's writing system
func (t *Translator) Direction() (direction string) {
	return t.rules.Direction
}

// substitute returns a string copy of the input str string will all keys in the
// substitutions map replaced with their value.
func (t *Translator) substitute(str string, substitutions map[string]string) (substituted string, errors []error) {

	substituted = str

	for find, replace := range substitutions {
		if strings.Index(str, "{"+find+"}") == -1 {
			errors = append(errors, translatorError{translator: t, message: "substitution not found: " + str + ", " + replace})
		}
		substituted = strings.Replace(substituted, "{"+find+"}", replace, -1)
	}

	return
}

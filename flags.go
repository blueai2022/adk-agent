package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/jessevdk/go-flags"
)

const (
	defaultKeyPath   = "api_key.txt"
	defaultModelName = "deepseek-v4.1-flash"
	defaultBaseURL   = "https://api.deepseek.com/v1"
)

// Flags holds the command line Flags for the application.
type Flags struct {
	Model   string `long:"model" description:"Model to use for the LLM service" required:"false" env:"LLM_MODEL"`
	APIKey  string `long:"api-key"  description:"API key for the LLM service" required:"false"  env:"LLM_API_KEY"`
	BaseURL string `long:"base-url" description:"Base URL for the LLM service" required:"false" env:"LLM_BASE_URL"`
}

// NewFlags creates a new Flags instance with the provided command line arguments.
//
//nolint:nilnil // return nil, nil is only markers for code readability, not actual return path
func NewFlags(args []string) (*Flags, error) {
	appFlags := &Flags{}

	parser := flags.NewParser(appFlags, flags.HelpFlag)

	_, err := parser.ParseArgs(args)
	if err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			fmt.Println(flagsErr)
			os.Exit(0)

			return nil, nil
		}

		return nil, fmt.Errorf("failed to parse flags: %v", err)
	}

	if err := appFlags.validate(); err != nil {
		return nil, fmt.Errorf("invalid flags: %v", err)
	}

	return appFlags, nil
}

// validate checks the Flags for required fields and returns an error if any are missing.
func (f *Flags) validate() error {
	var errs []error

	if f.resolveAPIKey() == "" {
		errs = append(errs, errors.New("api key is required"))
	}

	if f.Model == "" {
		f.Model = defaultModelName
	}

	if f.BaseURL == "" {
		f.BaseURL = defaultBaseURL
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

func (f *Flags) resolveAPIKey() string {
	if f.APIKey != "" {
		return f.APIKey
	}

	if bytes, err := os.ReadFile(defaultKeyPath); err == nil {
		f.APIKey = strings.TrimSpace(string(bytes))
	}

	return f.APIKey
}

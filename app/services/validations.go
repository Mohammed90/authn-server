package services

import (
	"fmt"
	"strings"

	"github.com/keratin/authn-server/app"
	"github.com/trustelem/zxcvbn"
)

var ErrMissing = "MISSING"
var ErrTaken = "TAKEN"
var ErrFormatInvalid = "FORMAT_INVALID"
var ErrInsecure = "INSECURE"
var ErrFailed = "FAILED"
var ErrLocked = "LOCKED"
var ErrExpired = "EXPIRED"
var ErrNotFound = "NOT_FOUND"
var ErrInvalidOrExpired = "INVALID_OR_EXPIRED"

type fieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e fieldError) String() string {
	return fmt.Sprintf("%v: %v", e.Field, e.Message)
}

type FieldErrors []fieldError

func (es FieldErrors) Error() string {
	var buf = make([]string, len(es))
	for i, e := range es {
		buf[i] = e.String()
	}
	return strings.Join(buf, ", ")
}

func passwordValidator(cfg *app.Config, password string) *fieldError {
	if password == "" {
		return &fieldError{"password", ErrMissing}
	}

	// SECURITY: only score the first 100 characters of a password. cheap benchmarks on my current
	//           laptop show that latency for 1e3 characters approaches 180ms, and 1e4 characters
	//           consume 54s.
	if len(password) > 100 {
		password = password[:100]
	}

	strength := zxcvbn.PasswordStrength(password, []string{})
	if strength.Score < cfg.PasswordMinComplexity {
		return &fieldError{"password", ErrInsecure}
	}

	return nil
}

func usernameValidator(cfg *app.Config, username string) *fieldError {
	if cfg.UsernameIsEmail {
		if !isEmail(username) {
			return &fieldError{"username", ErrFormatInvalid}
		}
		if len(cfg.UsernameDomains) > 0 && !hasDomain(username, cfg.UsernameDomains) {
			return &fieldError{"username", ErrFormatInvalid}
		}
	} else {
		if username == "" {
			return &fieldError{"username", ErrMissing}
		}
		if len(username) < cfg.UsernameMinLength {
			return &fieldError{"username", ErrFormatInvalid}
		}
	}
	return nil
}
